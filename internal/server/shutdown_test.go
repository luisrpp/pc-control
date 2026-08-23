package server_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/luisrpp/pc-control/internal/server"
)

type acceptanceSSHServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
	hostKey  ssh.PublicKey

	mu              sync.Mutex
	connections     int
	authentications int
	commands        []string
	command         chan struct{}
	close           sync.Once
}

type acceptanceSSHFixture struct {
	server         *acceptanceSSHServer
	privateKeyPath string
	knownHostsPath string
	port           uint16
}

func newAcceptanceSSHFixture(t *testing.T) *acceptanceSSHFixture {
	t.Helper()
	clientPublic, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test client key: %v", err)
	}
	clientKey, err := ssh.NewPublicKey(clientPublic)
	if err != nil {
		t.Fatalf("create test client public key: %v", err)
	}
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivate)
	if err != nil {
		t.Fatalf("create test host signer: %v", err)
	}

	fake := &acceptanceSSHServer{command: make(chan struct{})}
	fake.config = &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if string(key.Marshal()) != string(clientKey.Marshal()) {
				return nil, fmt.Errorf("reject non-test SSH key")
			}
			fake.mu.Lock()
			fake.authentications++
			fake.mu.Unlock()
			return nil, nil
		},
	}
	fake.config.AddHostKey(hostSigner)
	fake.hostKey = hostSigner.PublicKey()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback fake SSH server: %v", err)
	}
	fake.listener = listener
	go fake.serve()
	t.Cleanup(fake.Close)

	directory := t.TempDir()
	privateKeyPath := directory + "/client-key"
	der, err := x509.MarshalPKCS8PrivateKey(clientPrivate)
	if err != nil {
		t.Fatalf("marshal test client private key: %v", err)
	}
	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write test client private key: %v", err)
	}
	knownHostsPath := directory + "/known_hosts"
	line := knownhosts.Line([]string{fmt.Sprintf("[127.0.0.1]:%d", fake.Port())}, fake.hostKey)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write test known_hosts: %v", err)
	}

	return &acceptanceSSHFixture{server: fake, privateKeyPath: privateKeyPath, knownHostsPath: knownHostsPath, port: fake.Port()}
}

func (s *acceptanceSSHServer) serve() {
	for {
		connection, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.connections++
		s.mu.Unlock()
		go s.serveConnection(connection)
	}
}

func (s *acceptanceSSHServer) serveConnection(connection net.Conn) {
	defer connection.Close()
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
		go s.serveSession(channel, requests)
	}
}

func (s *acceptanceSSHServer) serveSession(channel ssh.Channel, requests <-chan *ssh.Request) {
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
		s.commands = append(s.commands, payload.Command)
		s.mu.Unlock()
		select {
		case <-s.command:
		default:
			close(s.command)
		}
		_ = request.Reply(true, nil)
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
		return
	}
}

func (s *acceptanceSSHServer) Port() uint16 {
	return uint16(s.listener.Addr().(*net.TCPAddr).Port)
}

func (s *acceptanceSSHServer) WaitForCommand(t *testing.T) {
	t.Helper()
	select {
	case <-s.command:
	case <-time.After(time.Second):
		t.Fatal("composed service did not reach the loopback fake SSH server")
	}
}

func (s *acceptanceSSHServer) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

func (s *acceptanceSSHServer) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

func (s *acceptanceSSHServer) Authentications() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authentications
}

func (s *acceptanceSSHServer) Close() {
	s.close.Do(func() { _ = s.listener.Close() })
}

func setValidShutdownEnvironment(t *testing.T, fixture *acceptanceSSHFixture) {
	t.Helper()
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_HOST", "127.0.0.1")
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PORT", fmt.Sprint(fixture.port))
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_USER", "pc-control-test")
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH", fixture.privateKeyPath)
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH", fixture.knownHostsPath)
	t.Setenv("PC_CONTROL_SHUTDOWN_TIMEOUT", "1s")
}

func setValidWakeEnvironmentForShutdown(t *testing.T, listenAddr string) {
	t.Helper()
	t.Setenv("PC_CONTROL_HTTP_LISTEN_ADDR", listenAddr)
	t.Setenv("PC_CONTROL_WOL_MAC", "02:00:00:00:00:01")
	t.Setenv("PC_CONTROL_WOL_DESTINATION", "127.0.0.1")
	t.Setenv("PC_CONTROL_WOL_PORT", "9")
}

func TestStartupRejectsInvalidShutdownConfigurationWithoutLeakingSensitiveValue(t *testing.T) {
	fixture := newAcceptanceSSHFixture(t)
	setValidWakeEnvironmentForShutdown(t, "127.0.0.1:8080")
	setValidShutdownEnvironment(t, fixture)
	const sensitivePrivateKeyPath = "PRIVATE-KEY-PATH-SECRET"
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH", sensitivePrivateKeyPath)

	service, err := server.NewFromEnv()
	if err == nil {
		t.Fatal("NewFromEnv() error = nil, want shutdown configuration failure")
	}
	if service != nil {
		t.Error("NewFromEnv() returned a service for invalid shutdown configuration")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "config") {
		t.Errorf("startup diagnostic = %q, want a configuration-failure indication", err)
	}
	if strings.Contains(err.Error(), sensitivePrivateKeyPath) {
		t.Errorf("startup diagnostic leaked sensitive shutdown configuration %q", sensitivePrivateKeyPath)
	}
}

func TestStartupRejectsIncompleteShutdownConfiguration(t *testing.T) {
	fixture := newAcceptanceSSHFixture(t)
	setValidWakeEnvironmentForShutdown(t, "127.0.0.1:8080")
	setValidShutdownEnvironment(t, fixture)
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_USER", "")

	service, err := server.NewFromEnv()
	if err == nil {
		t.Fatal("NewFromEnv() error = nil, want incomplete shutdown configuration failure")
	}
	if service != nil {
		t.Error("NewFromEnv() returned a service for incomplete shutdown configuration")
	}
}

func TestStartupRejectsInvalidShutdownKeyAndKnownHostsFiles(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
		set  func(*testing.T, string)
	}{
		{
			name: "unreadable private key path",
			path: "missing-private-key",
			set: func(t *testing.T, path string) {
				t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH", path)
			},
		},
		{
			name: "malformed private key data",
			path: "malformed-private-key",
			data: "not a private key",
			set: func(t *testing.T, path string) {
				t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH", path)
			},
		},
		{
			name: "encrypted private key data",
			path: "encrypted-private-key",
			data: encryptedTestPrivateKey(t),
			set: func(t *testing.T, path string) {
				t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH", path)
			},
		},
		{
			name: "unreadable known hosts path",
			path: "missing-known-hosts",
			set: func(t *testing.T, path string) {
				t.Setenv("PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH", path)
			},
		},
		{
			name: "malformed known hosts data",
			path: "malformed-known-hosts",
			data: "not known hosts data",
			set: func(t *testing.T, path string) {
				t.Setenv("PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH", path)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAcceptanceSSHFixture(t)
			setValidWakeEnvironmentForShutdown(t, "127.0.0.1:8080")
			setValidShutdownEnvironment(t, fixture)
			path := t.TempDir() + "/" + test.path
			if test.data != "" {
				if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
					t.Fatalf("write invalid test fixture: %v", err)
				}
			}
			test.set(t, path)

			service, err := server.NewFromEnv()
			if err == nil {
				t.Fatal("NewFromEnv() error = nil, want shutdown file configuration failure")
			}
			if service != nil {
				t.Error("NewFromEnv() returned a service for invalid shutdown file configuration")
			}
			if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), test.data) {
				t.Error("startup diagnostic disclosed shutdown file path or contents")
			}
		})
	}
}

func encryptedTestPrivateKey(t *testing.T) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate encrypted test private key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal encrypted test private key: %v", err)
	}
	block, err := x509.EncryptPEMBlock(rand.Reader, "PRIVATE KEY", der, []byte("test-passphrase"), x509.PEMCipherAES256)
	if err != nil {
		t.Fatalf("encrypt test private key: %v", err)
	}
	return string(pem.EncodeToMemory(block))
}

func TestFullCompositionShutdownAcceptanceUsesOnlyLoopbackFakeSSH(t *testing.T) {
	fixture := newAcceptanceSSHFixture(t)
	httpAddress := reserveTCPAddress(t)
	setValidWakeEnvironmentForShutdown(t, httpAddress)
	setValidShutdownEnvironment(t, fixture)

	service, err := server.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v, want valid composed service", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runResult := make(chan error, 1)
	go func() { runResult <- service.Run(ctx) }()

	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	retry := time.NewTicker(10 * time.Millisecond)
	defer retry.Stop()
	for {
		connection, dialErr := net.DialTimeout("tcp", httpAddress, 25*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("composed service did not start listening at %s: %v", httpAddress, dialErr)
		case <-retry.C:
		}
	}

	response, err := http.Post("http://"+httpAddress+"/v1/shutdown", "", nil)
	if err != nil {
		t.Fatalf("POST /v1/shutdown error = %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if response.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response body) error = %v", err)
	}
	if string(body) != `{"result":"initiated"}` {
		t.Errorf("response body = %q, want %q", body, `{"result":"initiated"}`)
	}
	fixture.server.WaitForCommand(t)
	if commands := fixture.server.Commands(); len(commands) != 1 || commands[0] != "systemctl poweroff" {
		t.Errorf("fake SSH server commands = %#v, want exactly [\"systemctl poweroff\"]", commands)
	}
	if got := fixture.server.Connections(); got != 1 {
		t.Errorf("fake SSH server connections = %d, want 1; acceptance must not retry", got)
	}
	if got := fixture.server.Authentications(); got != 1 {
		t.Errorf("fake SSH server accepted authentications = %d, want 1", got)
	}

	cancel()
	select {
	case <-runResult:
	case <-time.After(time.Second):
		t.Error("Run() did not return after cancellation")
	}
}
