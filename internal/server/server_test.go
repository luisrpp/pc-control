package server_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/luisrpp/pc-control/internal/server"
)

func setValidEnvironment(t *testing.T, listenAddr, destination string, port int) {
	t.Helper()
	t.Setenv("PC_CONTROL_HTTP_LISTEN_ADDR", listenAddr)
	t.Setenv("PC_CONTROL_WOL_MAC", "00:11:22:33:44:55")
	t.Setenv("PC_CONTROL_WOL_DESTINATION", destination)
	t.Setenv("PC_CONTROL_WOL_PORT", strconv.Itoa(port))
}

func TestStartupRejectsInvalidConfigurationWithoutLeakingSensitiveValue(t *testing.T) {
	const sensitiveMAC = "SECRET-WORKSTATION-MAC-IDENTIFIER"
	setValidEnvironment(t, "127.0.0.1:8080", "127.0.0.1", 9)
	t.Setenv("PC_CONTROL_WOL_MAC", sensitiveMAC)

	service, err := server.NewFromEnv()
	if err == nil {
		t.Fatal("NewFromEnv() error = nil, want configuration failure")
	}
	if service != nil {
		t.Error("NewFromEnv() returned a service for invalid configuration")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "config") {
		t.Errorf("startup diagnostic = %q, want a configuration-failure indication", err)
	}
	if strings.Contains(err.Error(), sensitiveMAC) {
		t.Errorf("startup diagnostic leaked sensitive configuration value %q", sensitiveMAC)
	}
}

func TestStartupRejectsIncompleteConfiguration(t *testing.T) {
	setValidEnvironment(t, "127.0.0.1:8080", "127.0.0.1", 9)
	t.Setenv("PC_CONTROL_WOL_DESTINATION", "")

	service, err := server.NewFromEnv()
	if err == nil {
		t.Fatal("NewFromEnv() error = nil, want configuration failure")
	}
	if service != nil {
		t.Error("NewFromEnv() returned a service for incomplete configuration")
	}
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved listener error = %v", err)
	}
	return address
}

func TestFullCompositionWakeAcceptance(t *testing.T) {
	shutdownFixture := newAcceptanceSSHFixture(t)
	udpReceiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP() error = %v", err)
	}
	t.Cleanup(func() { udpReceiver.Close() })

	udpPort := udpReceiver.LocalAddr().(*net.UDPAddr).Port
	httpAddress := reserveTCPAddress(t)
	setValidEnvironment(t, httpAddress, "127.0.0.1", udpPort)
	setValidShutdownEnvironment(t, shutdownFixture)

	service, err := server.NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v, want valid composed service", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runResult := make(chan error, 1)
	go func() { runResult <- service.Run(ctx) }()

	endpoint := "http://" + httpAddress + "/v1/wake"
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	retry := time.NewTicker(10 * time.Millisecond)
	defer retry.Stop()
	for {
		connection, dialErr := net.DialTimeout("tcp", httpAddress, 25*time.Millisecond)
		if dialErr == nil {
			connection.Close()
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("composed service did not start listening at %s: %v", httpAddress, dialErr)
		case <-retry.C:
		}
	}

	response, err := http.Post(endpoint, "", nil)
	if err != nil {
		t.Fatalf("POST /v1/wake error = %v", err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(response body) error = %v", err)
	}
	if string(body) != `{"result":"sent"}` {
		t.Errorf("response body = %q, want %q", body, `{"result":"sent"}`)
	}

	if err := udpReceiver.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	packet := make([]byte, 102)
	n, _, err := udpReceiver.ReadFromUDP(packet)
	if err != nil {
		t.Fatalf("ReadFromUDP() error = %v", err)
	}
	if n != 102 {
		t.Errorf("datagram size = %d, want 102", n)
	}
	if want := expectedMagicPacket([6]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}); string(packet) != string(want) {
		t.Errorf("datagram = %x, want exact Magic Packet %x", packet, want)
	}

	cancel()
	select {
	case <-runResult:
	case <-time.After(time.Second):
		t.Error("Run() did not return after cancellation")
	}
}

func expectedMagicPacket(mac [6]byte) []byte {
	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for i := 6; i < len(packet); i += len(mac) {
		copy(packet[i:], mac[:])
	}
	return packet
}
