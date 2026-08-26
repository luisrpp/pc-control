package tcpprobe_test

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/luisrpp/pc-control/internal/tcpprobe"
)

type connectionObservation struct {
	bytesRead int
	readErr   error
}

func TestAdapterConnectsOnceAndClosesWithoutSendingProtocolBytes(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	observed := make(chan connectionObservation, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			observed <- connectionObservation{readErr: err}
			return
		}
		var buffer [1]byte
		n, readErr := connection.Read(buffer[:])
		observed <- connectionObservation{bytesRead: n, readErr: readErr}
	}()

	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	adapter := tcpprobe.New(tcpprobe.Config{Host: "127.0.0.1", Port: port, Timeout: time.Second})
	if err := adapter.Probe(); err != nil {
		t.Errorf("Probe() error = %v, want nil after loopback TCP acceptance", err)
	}

	select {
	case got := <-observed:
		if got.bytesRead != 0 || got.readErr != io.EOF {
			t.Errorf("listener read = (%d, %v), want (0, EOF) after a closed TCP probe without payload", got.bytesRead, got.readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("loopback listener did not observe one probe connection")
	}
}

func TestAdapterReportsFailureForClosedLoopbackPort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatalf("close loopback listener error = %v", err)
	}

	adapter := tcpprobe.New(tcpprobe.Config{Host: "127.0.0.1", Port: port, Timeout: time.Second})
	if err := adapter.Probe(); err == nil {
		t.Error("Probe() error = nil, want failure for a closed loopback port")
	}
}
