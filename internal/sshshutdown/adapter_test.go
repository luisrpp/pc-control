package sshshutdown_test

import (
	"testing"
	"time"

	"github.com/luisrpp/pc-control/internal/sshshutdown"
)

func newTestAdapter(fixture *sshFixture, timeout time.Duration) *sshshutdown.Adapter {
	return sshshutdown.New(sshshutdown.Config{
		Host:           "127.0.0.1",
		Port:           fixture.port,
		User:           fixture.user,
		PrivateKeyPath: fixture.privateKeyPath,
		KnownHostsPath: fixture.knownHostsPath,
		Timeout:        timeout,
	})
}

func TestAdapterAuthenticatesWithDedicatedTestKeyAndRequestsFixedShutdownCommand(t *testing.T) {
	fixture := newSSHFixture(t, fakeSSHSuccess, false)
	adapter := newTestAdapter(fixture, time.Second)

	if err := adapter.Shutdown(); err != nil {
		t.Errorf("Shutdown() error = %v, want nil after the fake server accepts shutdown capability", err)
	}
	fixture.server.WaitForConnection(t)
	if got := fixture.server.Authentications(); got != 1 {
		t.Errorf("accepted test-key authentications = %d, want 1", got)
	}
	if commands := fixture.server.ExecCommands(); len(commands) != 1 || commands[0] != "systemctl poweroff" {
		t.Errorf("SSH exec commands = %#v, want exactly [\"systemctl poweroff\"]", commands)
	}
	if got := fixture.server.Connections(); got != 1 {
		t.Errorf("SSH connections = %d, want 1; successful operation must not retry", got)
	}
}

func TestAdapterRejectsUntrustedHostKeyBeforeRemoteOperation(t *testing.T) {
	fixture := newSSHFixture(t, fakeSSHSuccess, true)
	adapter := newTestAdapter(fixture, time.Second)

	if err := adapter.Shutdown(); err == nil {
		t.Error("Shutdown() error = nil, want host-key verification failure")
	}
	fixture.server.WaitForConnection(t)
	if commands := fixture.server.ExecCommands(); len(commands) != 0 {
		t.Errorf("SSH exec commands = %#v, want none after host-key rejection", commands)
	}
}

func TestAdapterReportsDedicatedKeyAuthenticationFailure(t *testing.T) {
	fixture := newSSHFixture(t, fakeSSHRejectAuthentication, false)
	adapter := newTestAdapter(fixture, time.Second)

	if err := adapter.Shutdown(); err == nil {
		t.Error("Shutdown() error = nil, want test-key authentication failure")
	}
	fixture.server.WaitForConnection(t)
	if commands := fixture.server.ExecCommands(); len(commands) != 0 {
		t.Errorf("SSH exec commands = %#v, want none after authentication failure", commands)
	}
}

func TestAdapterReportsRemoteOperationFailureWithoutRetry(t *testing.T) {
	fixture := newSSHFixture(t, fakeSSHCommandFailure, false)
	adapter := newTestAdapter(fixture, time.Second)

	if err := adapter.Shutdown(); err == nil {
		t.Error("Shutdown() error = nil, want remote operation failure")
	}
	fixture.server.WaitForConnection(t)
	if commands := fixture.server.ExecCommands(); len(commands) != 1 || commands[0] != "systemctl poweroff" {
		t.Errorf("SSH exec commands = %#v, want one fixed shutdown command", commands)
	}
	if got := fixture.server.Connections(); got != 1 {
		t.Errorf("SSH connections = %d, want 1; failed operations must not retry", got)
	}
}

func TestAdapterReportsIndeterminateDisconnectWithoutRetry(t *testing.T) {
	fixture := newSSHFixture(t, fakeSSHDisconnectAfterExec, false)
	adapter := newTestAdapter(fixture, time.Second)

	if err := adapter.Shutdown(); err == nil {
		t.Error("Shutdown() error = nil, want indeterminate connection-loss failure")
	}
	fixture.server.WaitForConnection(t)
	if commands := fixture.server.ExecCommands(); len(commands) != 1 || commands[0] != "systemctl poweroff" {
		t.Errorf("SSH exec commands = %#v, want one fixed shutdown command before disconnect", commands)
	}
	if got := fixture.server.Connections(); got != 1 {
		t.Errorf("SSH connections = %d, want 1; indeterminate failures must not retry", got)
	}
}

func TestAdapterAppliesEndToEndTimeoutToBlockedHandshake(t *testing.T) {
	fixture := newSSHFixture(t, fakeSSHBlockHandshake, false)
	adapter := newTestAdapter(fixture, 80*time.Millisecond)

	started := time.Now()
	err := adapter.Shutdown()
	elapsed := time.Since(started)
	if err == nil {
		t.Error("Shutdown() error = nil, want timeout failure")
	}
	fixture.server.WaitForConnection(t)
	if elapsed < 50*time.Millisecond || elapsed > time.Second {
		t.Errorf("Shutdown() elapsed = %v, want timeout-bound duration between 50ms and 1s", elapsed)
	}
	if commands := fixture.server.ExecCommands(); len(commands) != 0 {
		t.Errorf("SSH exec commands = %#v, want none while handshake is blocked", commands)
	}
}
