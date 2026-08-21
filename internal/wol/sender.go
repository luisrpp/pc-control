// Package wol contains the native UDP Wake-on-LAN adapter.
package wol

import (
	"errors"
	"net/netip"
)

var errNotImplemented = errors.New("pc-control WOL sender: not implemented")

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
	return errNotImplemented
}
