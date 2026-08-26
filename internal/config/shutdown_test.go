package config

import (
	"strings"
	"testing"
	"time"
)

const (
	shutdownSSHHost           = "PC_CONTROL_SHUTDOWN_SSH_HOST"
	shutdownSSHPort           = "PC_CONTROL_SHUTDOWN_SSH_PORT"
	shutdownSSHUser           = "PC_CONTROL_SHUTDOWN_SSH_USER"
	shutdownSSHPrivateKeyPath = "PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH"
	shutdownSSHKnownHostsPath = "PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH"
	shutdownTimeout           = "PC_CONTROL_SHUTDOWN_TIMEOUT"
)

func validShutdownRaw() map[string]string {
	return map[string]string{
		shutdownSSHHost:           "127.0.0.1",
		shutdownSSHUser:           "pc-control-test",
		shutdownSSHPrivateKeyPath: "/test/private-key",
		shutdownSSHKnownHostsPath: "/test/known-hosts",
	}
}

func TestParseShutdownAcceptsValuesAndDefaults(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]string
		want ShutdownConfig
	}{
		{
			name: "defaults",
			raw:  validShutdownRaw(),
			want: ShutdownConfig{
				SSHHost:            "127.0.0.1",
				SSHPort:            22,
				SSHUser:            "pc-control-test",
				SSHPrivateKeyPath:  "/test/private-key",
				SSHKnownHostsPath:  "/test/known-hosts",
				SSHTimeout:         10 * time.Second,
				StatusProbeTimeout: time.Second,
			},
		},
		{
			name: "hostname IPv6 and explicit values",
			raw: map[string]string{
				shutdownSSHHost:           "shutdown.test",
				shutdownSSHPort:           "022",
				shutdownSSHUser:           "pc-control-test",
				shutdownSSHPrivateKeyPath: "/test/private-key",
				shutdownSSHKnownHostsPath: "/test/known-hosts",
				shutdownTimeout:           "1500ms",
			},
			want: ShutdownConfig{
				SSHHost:            "shutdown.test",
				SSHPort:            22,
				SSHUser:            "pc-control-test",
				SSHPrivateKeyPath:  "/test/private-key",
				SSHKnownHostsPath:  "/test/known-hosts",
				SSHTimeout:         1500 * time.Millisecond,
				StatusProbeTimeout: time.Second,
			},
		},
		{
			name: "IPv6 literal",
			raw: map[string]string{
				shutdownSSHHost:           "2001:db8::10",
				shutdownSSHPort:           "65535",
				shutdownSSHUser:           "pc-control-test",
				shutdownSSHPrivateKeyPath: "/test/private-key",
				shutdownSSHKnownHostsPath: "/test/known-hosts",
				shutdownTimeout:           "1s",
			},
			want: ShutdownConfig{
				SSHHost:            "2001:db8::10",
				SSHPort:            65535,
				SSHUser:            "pc-control-test",
				SSHPrivateKeyPath:  "/test/private-key",
				SSHKnownHostsPath:  "/test/known-hosts",
				SSHTimeout:         time.Second,
				StatusProbeTimeout: time.Second,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseShutdown(test.raw)
			if err != nil {
				t.Fatalf("parseShutdown() error = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("parseShutdown() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseShutdownRejectsMissingEmptyAndWhitespacePaddedRequiredValues(t *testing.T) {
	for _, key := range []string{shutdownSSHHost, shutdownSSHUser, shutdownSSHPrivateKeyPath, shutdownSSHKnownHostsPath} {
		t.Run("missing "+key, func(t *testing.T) {
			raw := validShutdownRaw()
			delete(raw, key)
			if _, err := parseShutdown(raw); err == nil {
				t.Errorf("parseShutdown() error = nil, want error for missing %s", key)
			}
		})

		for _, value := range []string{"", " value", "value ", "\tvalue"} {
			t.Run("invalid "+key+" value "+strings.ReplaceAll(value, " ", "space"), func(t *testing.T) {
				raw := validShutdownRaw()
				raw[key] = value
				if _, err := parseShutdown(raw); err == nil {
					t.Errorf("parseShutdown() error = nil, want error for %s=%q", key, value)
				}
			})
		}
	}
}

func TestParseShutdownRejectsInvalidSSHHosts(t *testing.T) {
	for _, host := range []string{"", " host", "host ", "ssh://shutdown.test", "shutdown.test:22", "[::1]", "[::1]:22"} {
		t.Run(host, func(t *testing.T) {
			raw := validShutdownRaw()
			raw[shutdownSSHHost] = host
			if _, err := parseShutdown(raw); err == nil {
				t.Errorf("parseShutdown() error = nil, want error for SSH host %q", host)
			}
		})
	}
}

func TestParseShutdownRejectsInvalidSSHPorts(t *testing.T) {
	for _, port := range []string{"", " ", " 22", "22 ", "twenty-two", "0", "-1", "65536", "22.0"} {
		t.Run(port, func(t *testing.T) {
			raw := validShutdownRaw()
			raw[shutdownSSHPort] = port
			if _, err := parseShutdown(raw); err == nil {
				t.Errorf("parseShutdown() error = nil, want error for SSH port %q", port)
			}
		})
	}
}

func TestParseShutdownRejectsInvalidTimeouts(t *testing.T) {
	for _, timeout := range []string{"", " ", " 1s", "1s ", "one second", "0", "0s", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			raw := validShutdownRaw()
			raw[shutdownTimeout] = timeout
			if _, err := parseShutdown(raw); err == nil {
				t.Errorf("parseShutdown() error = nil, want error for timeout %q", timeout)
			}
		})
	}
}
