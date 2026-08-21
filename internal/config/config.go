// Package config loads pc-control runtime configuration.
package config

import (
	"errors"
	"net/netip"
)

var errNotImplemented = errors.New("pc-control configuration: not implemented")

// Config is the validated runtime configuration used by composition.
type Config struct {
	HTTPListenAddr string
	WOLMAC         [6]byte
	WOLDestination netip.Addr
	WOLPort        uint16
}

// LoadFromEnv loads runtime configuration from the process environment.
func LoadFromEnv() (Config, error) {
	return Config{}, errNotImplemented
}

// parse validates the four raw configuration values. A missing map key means
// that the corresponding environment variable is absent.
func parse(raw map[string]string) (Config, error) {
	return Config{}, errNotImplemented
}
