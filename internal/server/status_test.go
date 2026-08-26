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
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/luisrpp/pc-control/internal/server"
)

type statusShutdownMaterial struct {
	privateKeyPath string
	knownHostsPath string
}

type statusConnectionObservation struct {
	bytesRead int
	readErr   error
}

func newStatusShutdownMaterial(t *testing.T, port uint16) statusShutdownMaterial {
	t.Helper()
	_, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test client key: %v", err)
	}
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivate)
	if err != nil {
		t.Fatalf("create test host signer: %v", err)
	}

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
	line := knownhosts.Line([]string{fmt.Sprintf("[127.0.0.1]:%d", port)}, hostSigner.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write test known_hosts: %v", err)
	}
	return statusShutdownMaterial{privateKeyPath: privateKeyPath, knownHostsPath: knownHostsPath}
}

func setValidStatusEnvironment(t *testing.T, httpAddress string, statusPort uint16, material statusShutdownMaterial) {
	t.Helper()
	t.Setenv("PC_CONTROL_HTTP_LISTEN_ADDR", httpAddress)
	t.Setenv("PC_CONTROL_WOL_MAC", "02:00:00:00:00:01")
	t.Setenv("PC_CONTROL_WOL_DESTINATION", "127.0.0.1")
	t.Setenv("PC_CONTROL_WOL_PORT", "9")
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_HOST", "127.0.0.1")
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PORT", fmt.Sprint(statusPort))
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_USER", "pc-control-test")
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH", material.privateKeyPath)
	t.Setenv("PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH", material.knownHostsPath)
	t.Setenv("PC_CONTROL_SHUTDOWN_TIMEOUT", "1s")
}

func TestStartupRejectsInvalidStatusProbeTimeoutWithoutLeakingSensitiveValue(t *testing.T) {
	material := newStatusShutdownMaterial(t, 22)
	setValidStatusEnvironment(t, "127.0.0.1:8080", 22, material)
	const sensitiveTimeout = "STATUS-PROBE-TIMEOUT-SECRET"
	t.Setenv("PC_CONTROL_STATUS_PROBE_TIMEOUT", sensitiveTimeout)

	service, err := server.NewFromEnv()
	if err == nil {
		t.Fatal("NewFromEnv() error = nil, want status timeout configuration failure")
	}
	if service != nil {
		t.Error("NewFromEnv() returned a service for invalid status timeout configuration")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "config") {
		t.Errorf("startup diagnostic = %q, want a configuration-failure indication", err)
	}
	if strings.Contains(err.Error(), sensitiveTimeout) {
		t.Errorf("startup diagnostic leaked sensitive status timeout value %q", sensitiveTimeout)
	}
}

func TestFullCompositionStatusAcceptanceUsesOnlyLoopbackTCP(t *testing.T) {
	probeListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for loopback status probe: %v", err)
	}
	t.Cleanup(func() { _ = probeListener.Close() })
	probePort := uint16(probeListener.Addr().(*net.TCPAddr).Port)
	material := newStatusShutdownMaterial(t, probePort)

	udpReceiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen for loopback WOL datagrams: %v", err)
	}
	t.Cleanup(func() { _ = udpReceiver.Close() })

	httpAddress := reserveTCPAddress(t)
	setValidStatusEnvironment(t, httpAddress, probePort, material)
	t.Setenv("PC_CONTROL_WOL_PORT", fmt.Sprint(udpReceiver.LocalAddr().(*net.UDPAddr).Port))

	probeObserved := make(chan statusConnectionObservation, 1)
	go func() {
		connection, acceptErr := probeListener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			probeObserved <- statusConnectionObservation{readErr: err}
			return
		}
		var buffer [1]byte
		n, readErr := connection.Read(buffer[:])
		probeObserved <- statusConnectionObservation{bytesRead: n, readErr: readErr}
	}()

	service, err := server.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v, want valid composed service", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

	response, err := http.Get("http://" + httpAddress + "/v1/status")
	if err != nil {
		t.Fatalf("GET /v1/status error = %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response body) error = %v", err)
	}
	if string(body) != `{"status":"online"}` {
		t.Errorf("response body = %q, want %q", body, `{"status":"online"}`)
	}

	select {
	case got := <-probeObserved:
		if got.bytesRead != 0 || got.readErr != io.EOF {
			t.Errorf("status listener read = (%d, %v), want (0, EOF) without SSH or application payload", got.bytesRead, got.readErr)
		}
	case <-time.After(time.Second):
		t.Error("status listener did not observe the configured loopback TCP probe")
	}

	if err := udpReceiver.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("set WOL receiver deadline: %v", err)
	}
	var datagram [1]byte
	if n, _, err := udpReceiver.ReadFromUDP(datagram[:]); err == nil || n != 0 {
		t.Errorf("status request unexpectedly emitted a WOL datagram of %d bytes", n)
	}

	cancel()
	select {
	case <-runResult:
	case <-time.After(time.Second):
		t.Error("Run() did not return after cancellation")
	}
}
