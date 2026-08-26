// Package config loads pc-control runtime configuration.
package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	envHTTPListenAddr            = "PC_CONTROL_HTTP_LISTEN_ADDR"
	envWOLMAC                    = "PC_CONTROL_WOL_MAC"
	envWOLDestination            = "PC_CONTROL_WOL_DESTINATION"
	envWOLPort                   = "PC_CONTROL_WOL_PORT"
	envShutdownSSHHost           = "PC_CONTROL_SHUTDOWN_SSH_HOST"
	envShutdownSSHPort           = "PC_CONTROL_SHUTDOWN_SSH_PORT"
	envShutdownSSHUser           = "PC_CONTROL_SHUTDOWN_SSH_USER"
	envShutdownSSHPrivateKeyPath = "PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH"
	envShutdownSSHKnownHostsPath = "PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH"
	envShutdownTimeout           = "PC_CONTROL_SHUTDOWN_TIMEOUT"
	envStatusProbeTimeout        = "PC_CONTROL_STATUS_PROBE_TIMEOUT"
)

// Config is the validated runtime configuration used by composition.
type Config struct {
	HTTPListenAddr string
	WOLMAC         [6]byte
	WOLDestination netip.Addr
	WOLPort        uint16
}

// ShutdownConfig holds graceful-shutdown configuration for composition.
type ShutdownConfig struct {
	SSHHost            string
	SSHPort            uint16
	SSHUser            string
	SSHPrivateKeyPath  string
	SSHKnownHostsPath  string
	SSHTimeout         time.Duration
	StatusProbeTimeout time.Duration
}

// LoadFromEnv loads runtime configuration from the process environment.
func LoadFromEnv() (Config, error) {
	raw := make(map[string]string, 4)
	for _, key := range []string{envHTTPListenAddr, envWOLMAC, envWOLDestination, envWOLPort} {
		if value, ok := os.LookupEnv(key); ok {
			raw[key] = value
		}
	}
	return parse(raw)
}

// LoadShutdownFromEnv loads graceful-shutdown configuration from the process
// environment.
func LoadShutdownFromEnv() (ShutdownConfig, error) {
	raw := make(map[string]string, 7)
	for _, key := range []string{
		envShutdownSSHHost,
		envShutdownSSHPort,
		envShutdownSSHUser,
		envShutdownSSHPrivateKeyPath,
		envShutdownSSHKnownHostsPath,
		envShutdownTimeout,
		envStatusProbeTimeout,
	} {
		if value, ok := os.LookupEnv(key); ok {
			raw[key] = value
		}
	}
	return parseShutdown(raw)
}

// parseShutdown validates the raw graceful-shutdown configuration values. A
// missing map key means that the corresponding environment variable is absent.
func parseShutdown(raw map[string]string) (ShutdownConfig, error) {
	host, err := required(raw, envShutdownSSHHost)
	if err != nil {
		return ShutdownConfig{}, err
	}
	if err := validateSSHHost(host); err != nil {
		return ShutdownConfig{}, err
	}

	user, err := required(raw, envShutdownSSHUser)
	if err != nil {
		return ShutdownConfig{}, err
	}
	privateKeyPath, err := required(raw, envShutdownSSHPrivateKeyPath)
	if err != nil {
		return ShutdownConfig{}, err
	}
	knownHostsPath, err := required(raw, envShutdownSSHKnownHostsPath)
	if err != nil {
		return ShutdownConfig{}, err
	}

	port := uint16(22)
	if portText, ok := raw[envShutdownSSHPort]; ok {
		if err := validateNoOuterWhitespace(portText, envShutdownSSHPort); err != nil || portText == "" {
			return ShutdownConfig{}, fmt.Errorf("pc-control configuration: invalid %s", envShutdownSSHPort)
		}
		port, err = parsePort(portText, envShutdownSSHPort)
		if err != nil {
			return ShutdownConfig{}, err
		}
	}

	timeout := 10 * time.Second
	if timeoutText, ok := raw[envShutdownTimeout]; ok {
		if err := validateNoOuterWhitespace(timeoutText, envShutdownTimeout); err != nil || timeoutText == "" {
			return ShutdownConfig{}, fmt.Errorf("pc-control configuration: invalid %s", envShutdownTimeout)
		}
		timeout, err = time.ParseDuration(timeoutText)
		if err != nil || timeout <= 0 {
			return ShutdownConfig{}, fmt.Errorf("pc-control configuration: invalid %s", envShutdownTimeout)
		}
	}

	return ShutdownConfig{
		SSHHost:            host,
		SSHPort:            port,
		SSHUser:            user,
		SSHPrivateKeyPath:  privateKeyPath,
		SSHKnownHostsPath:  knownHostsPath,
		SSHTimeout:         timeout,
		StatusProbeTimeout: 0,
	}, nil
}

// parse validates the four raw configuration values. A missing map key means
// that the corresponding environment variable is absent.
func parse(raw map[string]string) (Config, error) {
	listenAddr, err := required(raw, envHTTPListenAddr)
	if err != nil {
		return Config{}, err
	}
	if err := validateListenAddr(listenAddr); err != nil {
		return Config{}, err
	}

	macText, err := required(raw, envWOLMAC)
	if err != nil {
		return Config{}, err
	}
	mac, err := parseMAC(macText)
	if err != nil {
		return Config{}, err
	}

	destinationText, err := required(raw, envWOLDestination)
	if err != nil {
		return Config{}, err
	}
	destination, err := parseIPv4(destinationText)
	if err != nil {
		return Config{}, err
	}

	port := uint16(9)
	if portText, ok := raw[envWOLPort]; ok {
		if err := validateNoOuterWhitespace(portText, envWOLPort); err != nil || portText == "" {
			return Config{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLPort)
		}
		port, err = parsePort(portText, envWOLPort)
		if err != nil {
			return Config{}, err
		}
	}

	return Config{
		HTTPListenAddr: listenAddr,
		WOLMAC:         mac,
		WOLDestination: destination,
		WOLPort:        port,
	}, nil
}

func required(raw map[string]string, key string) (string, error) {
	value, ok := raw[key]
	if !ok || value == "" {
		return "", fmt.Errorf("pc-control configuration: missing required %s", key)
	}
	if err := validateNoOuterWhitespace(value, key); err != nil {
		return "", err
	}
	return value, nil
}

func validateNoOuterWhitespace(value, key string) error {
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("pc-control configuration: invalid %s", key)
	}
	return nil
}

func validateListenAddr(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("pc-control configuration: invalid %s", envHTTPListenAddr)
	}
	if _, err := parsePort(portText, envHTTPListenAddr); err != nil {
		return err
	}
	if host == "" {
		return nil
	}

	address, err := netip.ParseAddr(host)
	if err != nil || address.Zone() != "" || (address.Is4() && strings.HasPrefix(value, "[")) {
		return fmt.Errorf("pc-control configuration: invalid %s", envHTTPListenAddr)
	}
	return nil
}

func validateSSHHost(value string) error {
	if address, err := netip.ParseAddr(value); err == nil && address.Zone() == "" {
		return nil
	}

	if strings.ContainsAny(value, ":/[]") || !validHostname(value) {
		return fmt.Errorf("pc-control configuration: invalid %s", envShutdownSSHHost)
	}
	return nil
}

func validHostname(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	if strings.HasSuffix(value, ".") {
		value = strings.TrimSuffix(value, ".")
	}
	if value == "" {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') &&
				(character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') &&
				character != '-' {
				return false
			}
		}
	}
	return true
}

func parsePort(value, key string) (uint16, error) {
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("pc-control configuration: invalid %s", key)
	}
	return uint16(parsed), nil
}

func parseIPv4(value string) (netip.Addr, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return netip.Addr{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLDestination)
	}

	var octets [4]byte
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return netip.Addr{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLDestination)
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return netip.Addr{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLDestination)
			}
		}
		octet, err := strconv.ParseUint(part, 10, 8)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLDestination)
		}
		octets[i] = byte(octet)
	}
	return netip.AddrFrom4(octets), nil
}

func parseMAC(value string) ([6]byte, error) {
	var compact string
	switch {
	case len(value) == 17 && (value[2] == ':' || value[2] == '-'):
		separator := value[2]
		for _, offset := range []int{5, 8, 11, 14} {
			if value[offset] != separator {
				return [6]byte{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLMAC)
			}
		}
		compact = strings.ReplaceAll(value, string(separator), "")
	case len(value) == 14 && value[4] == '.' && value[9] == '.':
		compact = strings.ReplaceAll(value, ".", "")
	default:
		return [6]byte{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLMAC)
	}

	var mac [6]byte
	if decoded, err := hex.Decode(mac[:], []byte(compact)); err != nil || decoded != len(mac) {
		return [6]byte{}, fmt.Errorf("pc-control configuration: invalid %s", envWOLMAC)
	}
	return mac, nil
}
