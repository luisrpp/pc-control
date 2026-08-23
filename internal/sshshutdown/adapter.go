// Package sshshutdown contains the SSH adapter boundary for graceful shutdown.
package sshshutdown

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const shutdownCommand = "systemctl poweroff"

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
	config           Config
	signer           ssh.Signer
	hostKeyCallback  ssh.HostKeyCallback
	configurationErr error
}

// New creates an SSH shutdown adapter.
func New(config Config) *Adapter {
	signer, hostKeyCallback, err := loadCredentials(config)
	return &Adapter{
		config:           config,
		signer:           signer,
		hostKeyCallback:  hostKeyCallback,
		configurationErr: err,
	}
}

// Validate reports whether the configured private key and known-hosts data can
// be used by the adapter. Composition calls this before the service starts.
func (a *Adapter) Validate() error {
	return a.configurationErr
}

// Shutdown performs one SSH shutdown operation using the fixed graceful
// shutdown capability command.
func (a *Adapter) Shutdown() error {
	if a == nil || a.configurationErr != nil {
		return fmt.Errorf("SSH shutdown adapter is not configured")
	}

	deadline := time.Now().Add(a.config.Timeout)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	address := net.JoinHostPort(a.config.Host, strconv.FormatUint(uint64(a.config.Port), 10))
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect SSH shutdown target: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SSH shutdown deadline: %w", err)
	}

	clientConnection, channels, requests, err := ssh.NewClientConn(connection, address, &ssh.ClientConfig{
		User:            a.config.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(a.signer)},
		HostKeyCallback: a.hostKeyCallback,
	})
	if err != nil {
		return fmt.Errorf("establish SSH shutdown session: %w", err)
	}
	client := ssh.NewClient(clientConnection, channels, requests)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH shutdown operation: %w", err)
	}
	defer session.Close()
	if err := session.Run(shutdownCommand); err != nil {
		return fmt.Errorf("run SSH shutdown operation: %w", err)
	}
	return nil
}

func loadCredentials(config Config) (ssh.Signer, ssh.HostKeyCallback, error) {
	privateKey, err := os.ReadFile(config.PrivateKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read SSH private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("parse SSH private key: %w", err)
	}
	hostKeyCallback, err := knownhosts.New(config.KnownHostsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load SSH known hosts: %w", err)
	}
	return signer, hostKeyCallback, nil
}
