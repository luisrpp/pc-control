// Package tcpprobe contains the TCP adapter boundary for workstation status.
package tcpprobe

import (
	"errors"
	"time"
)

// Config supplies the TCP probe adapter's deployment settings.
type Config struct {
	Host    string
	Port    uint16
	Timeout time.Duration
}

// Adapter is the TCP implementation of the status probe boundary.
type Adapter struct {
	config Config
}

// New creates a TCP probe adapter.
func New(config Config) *Adapter {
	return &Adapter{config: config}
}

// Probe performs one TCP reachability probe.
//
// TCP dialing behavior is implemented in the following production phase.
func (a *Adapter) Probe() error {
	return errors.New("TCP status probe is not implemented")
}
