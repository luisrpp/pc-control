// Package tcpprobe contains the TCP adapter boundary for workstation status.
package tcpprobe

import (
	"net"
	"strconv"
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
func (a *Adapter) Probe() error {
	address := net.JoinHostPort(a.config.Host, strconv.FormatUint(uint64(a.config.Port), 10))
	connection, err := (&net.Dialer{Timeout: a.config.Timeout}).Dial("tcp", address)
	if err != nil {
		return err
	}
	defer connection.Close()
	return nil
}
