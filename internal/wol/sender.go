// Package wol contains the native UDP Wake-on-LAN adapter.
package wol

import (
	"fmt"
	"io"
	"net"
	"net/netip"
)

// Sender sends a Wake-on-LAN magic packet to one configured destination.
type Sender struct {
	destination netip.Addr
	port        uint16
	mac         [6]byte
}

// NewSender creates a native UDP Wake-on-LAN sender.
func NewSender(destination netip.Addr, port uint16, mac [6]byte) *Sender {
	return &Sender{destination: destination, port: port, mac: mac}
}

// Send sends one Wake-on-LAN magic packet.
func (s *Sender) Send() error {
	packet := magicPacket(s.mac)
	destination := net.UDPAddrFromAddrPort(netip.AddrPortFrom(s.destination, s.port))
	connection, err := net.DialUDP("udp4", nil, destination)
	if err != nil {
		return fmt.Errorf("dial WOL destination: %w", err)
	}
	defer connection.Close()

	written, err := connection.Write(packet[:])
	if err != nil {
		return fmt.Errorf("send WOL magic packet: %w", err)
	}
	if written != len(packet) {
		return io.ErrShortWrite
	}
	return nil
}

func magicPacket(mac [6]byte) [102]byte {
	var packet [102]byte
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for offset := 6; offset < len(packet); offset += len(mac) {
		copy(packet[offset:], mac[:])
	}
	return packet
}
