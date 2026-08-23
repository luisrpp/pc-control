package sshshutdown_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type fakeSSHMode int

const (
	fakeSSHSuccess fakeSSHMode = iota
	fakeSSHRejectAuthentication
	fakeSSHCommandFailure
	fakeSSHDisconnectAfterExec
	fakeSSHBlockHandshake
)

type fakeSSHServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
	mode     fakeSSHMode
	hostKey  ssh.PublicKey

	mu              sync.Mutex
	connections     int
	authentications int
	execCommands    []string

	connected chan struct{}
	release   chan struct{}
	closeOnce sync.Once
}

type sshFixture struct {
	server         *fakeSSHServer
	privateKeyPath string
	knownHostsPath string
	port           uint16
	user           string
}

func newSSHFixture(t *testing.T, mode fakeSSHMode, wrongKnownHost bool) *sshFixture {
	t.Helper()

	clientPublic, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test client key: %v", err)
	}
	clientKey, err := ssh.NewPublicKey(clientPublic)
	if err != nil {
		t.Fatalf("create test client public key: %v", err)
	}
	server := newFakeSSHServer(t, mode, clientKey)

	directory := t.TempDir()
	privateKeyPath := directory + "/client-key"
	writeTestPrivateKey(t, privateKeyPath, clientPrivate)

	hostPublic := server.hostKey
	if wrongKnownHost {
		wrongPublic, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate wrong host key: %v", err)
		}
		hostPublic, err = ssh.NewPublicKey(wrongPublic)
		if err != nil {
			t.Fatalf("create wrong host public key: %v", err)
		}
	}
	knownHostsPath := directory + "/known_hosts"
	line := knownhosts.Line([]string{fmt.Sprintf("[127.0.0.1]:%d", server.Port())}, hostPublic)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write test known_hosts: %v", err)
	}

	return &sshFixture{
		server:         server,
		privateKeyPath: privateKeyPath,
		knownHostsPath: knownHostsPath,
		port:           server.Port(),
		user:           "pc-control-test",
	}
}

func writeTestPrivateKey(t *testing.T, path string, privateKey ed25519.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal test private key: %v", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write test private key: %v", err)
	}
}

func newFakeSSHServer(t *testing.T, mode fakeSSHMode, clientKey ssh.PublicKey) *fakeSSHServer {
	t.Helper()
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivate)
	if err != nil {
		t.Fatalf("create test host signer: %v", err)
	}
	server := &fakeSSHServer{
		mode:      mode,
		connected: make(chan struct{}),
		release:   make(chan struct{}),
	}
	server.config = &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if mode == fakeSSHRejectAuthentication || string(key.Marshal()) != string(clientKey.Marshal()) {
				return nil, errors.New("test SSH authentication rejected")
			}
			server.mu.Lock()
			server.authentications++
			server.mu.Unlock()
			return nil, nil
		},
	}
	server.config.AddHostKey(hostSigner)
	server.hostKey = hostSigner.PublicKey()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback fake SSH server: %v", err)
	}
	server.listener = listener

	go server.serve()
	t.Cleanup(server.Close)
	return server
}

func (s *fakeSSHServer) serve() {
	for {
		connection, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.connections++
		s.mu.Unlock()
		select {
		case <-s.connected:
		default:
			close(s.connected)
		}
		go s.serveConnection(connection)
	}
}

func (s *fakeSSHServer) serveConnection(connection net.Conn) {
	defer connection.Close()
	if s.mode == fakeSSHBlockHandshake {
		<-s.release
		return
	}

	serverConnection, channels, requests, err := ssh.NewServerConn(connection, s.config)
	if err != nil {
		return
	}
	defer serverConnection.Close()
	go ssh.DiscardRequests(requests)
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "test server accepts sessions only")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go s.serveSession(channel, requests, serverConnection)
	}
}

func (s *fakeSSHServer) serveSession(channel ssh.Channel, requests <-chan *ssh.Request, serverConnection *ssh.ServerConn) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			_ = request.Reply(false, nil)
			return
		}
		s.mu.Lock()
		s.execCommands = append(s.execCommands, payload.Command)
		s.mu.Unlock()
		_ = request.Reply(true, nil)

		switch s.mode {
		case fakeSSHSuccess:
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
		case fakeSSHCommandFailure:
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 1}))
		case fakeSSHDisconnectAfterExec:
			_ = serverConnection.Close()
		}
		return
	}
}

func (s *fakeSSHServer) Port() uint16 {
	return uint16(s.listener.Addr().(*net.TCPAddr).Port)
}

func (s *fakeSSHServer) WaitForConnection(t *testing.T) {
	t.Helper()
	select {
	case <-s.connected:
	case <-time.After(time.Second):
		t.Fatal("SSH adapter did not connect to the loopback fake server")
	}
}

func (s *fakeSSHServer) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

func (s *fakeSSHServer) Authentications() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authentications
}

func (s *fakeSSHServer) ExecCommands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.execCommands...)
}

func (s *fakeSSHServer) Close() {
	s.closeOnce.Do(func() {
		close(s.release)
		_ = s.listener.Close()
	})
}
