package wol_test

import (
	"bytes"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/luisrpp/pc-control/internal/wol"
)

func magicPacket(mac [6]byte) []byte {
	packet := make([]byte, 0, 102)
	packet = append(packet, bytes.Repeat([]byte{0xff}, 6)...)
	for i := 0; i < 16; i++ {
		packet = append(packet, mac[:]...)
	}
	return packet
}

func TestSenderEmitsOneExactMagicPacketToLoopbackReceiver(t *testing.T) {
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP() error = %v", err)
	}
	t.Cleanup(func() { receiver.Close() })

	port := receiver.LocalAddr().(*net.UDPAddr).Port
	mac := [6]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	sender := wol.NewSender(netip.MustParseAddr("127.0.0.1"), uint16(port), mac)

	if err := sender.Send(); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}

	if err := receiver.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	packet := make([]byte, 102)
	n, _, err := receiver.ReadFromUDP(packet)
	if err != nil {
		t.Fatalf("ReadFromUDP() error = %v", err)
	}
	if n != 102 {
		t.Fatalf("datagram size = %d, want 102", n)
	}
	if want := magicPacket(mac); !bytes.Equal(packet, want) {
		t.Errorf("datagram = %x, want exact Magic Packet %x", packet, want)
	}
}
