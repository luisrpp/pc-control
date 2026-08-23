// Package sshshutdown contains the SSH adapter boundary for graceful shutdown.
package sshshutdown

import (
	"errors"
	"time"
)

// ErrNotImplemented marks the temporary SSH adapter scaffold.
var ErrNotImplemented = errors.New("SSH graceful shutdown is not implemented")

// Config supplies the SSH adapter's deployment settings.
type Config struct {
	Host           string
	Port           uint16
	User           string
	PrivateKeyPath string
	KnownHostsPath string
	Timeout        time.Duration
}

// Adapter is the SSH implementation of the shutdown boundary.
type Adapter struct {
	config Config
}

// New creates an SSH shutdown adapter.
func New(config Config) *Adapter {
	return &Adapter{config: config}
}

// Shutdown is a structural placeholder until SSH shutdown behavior is
// implemented.
func (a *Adapter) Shutdown() error {
	return ErrNotImplemented
}
