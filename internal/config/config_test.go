package config

import (
	"net/netip"
	"strings"
	"testing"
)

const (
	httpListenAddr = "PC_CONTROL_HTTP_LISTEN_ADDR"
	wolMAC         = "PC_CONTROL_WOL_MAC"
	wolDestination = "PC_CONTROL_WOL_DESTINATION"
	wolPort        = "PC_CONTROL_WOL_PORT"
)

func validRaw() map[string]string {
	return map[string]string{
		httpListenAddr: "127.0.0.1:8080",
		wolMAC:         "aa:bb:cc:dd:ee:ff",
		wolDestination: "192.168.1.255",
	}
}

func TestParseAcceptsValidConfiguration(t *testing.T) {
	mac := [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	tests := []struct {
		name string
		raw  map[string]string
		want Config
	}{
		{
			name: "colon MAC and default port",
			raw:  validRaw(),
			want: Config{"127.0.0.1:8080", mac, netip.MustParseAddr("192.168.1.255"), 9},
		},
		{
			name: "hyphen MAC with uppercase hexadecimal and leading-zero port",
			raw: map[string]string{
				httpListenAddr: "[::1]:8080",
				wolMAC:         "AA-BB-CC-DD-EE-FF",
				wolDestination: "127.0.0.1",
				wolPort:        "09",
			},
			want: Config{"[::1]:8080", mac, netip.MustParseAddr("127.0.0.1"), 9},
		},
		{
			name: "dotted MAC and wildcard listen address",
			raw: map[string]string{
				httpListenAddr: ":65535",
				wolMAC:         "aabb.ccdd.eeff",
				wolDestination: "255.255.255.255",
				wolPort:        "65535",
			},
			want: Config{":65535", mac, netip.MustParseAddr("255.255.255.255"), 65535},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parse(test.raw)
			if err != nil {
				t.Fatalf("parse() error = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("parse() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseRejectsMissingEmptyAndWhitespacePaddedRequiredValues(t *testing.T) {
	for _, key := range []string{httpListenAddr, wolMAC, wolDestination} {
		t.Run("missing "+key, func(t *testing.T) {
			raw := validRaw()
			delete(raw, key)
			if _, err := parse(raw); err == nil {
				t.Errorf("parse() error = nil, want error for missing %s", key)
			}
		})

		for _, value := range []string{"", " value", "value ", "\tvalue"} {
			t.Run("invalid "+key+" value "+strings.ReplaceAll(value, " ", "space"), func(t *testing.T) {
				raw := validRaw()
				raw[key] = value
				if _, err := parse(raw); err == nil {
					t.Errorf("parse() error = nil, want error for %s=%q", key, value)
				}
			})
		}
	}
}

func TestParseRejectsInvalidListenAddresses(t *testing.T) {
	for _, listenAddr := range []string{
		"localhost:8080", "example.com:8080", "127.0.0.1", "127.0.0.1:",
		"127.0.0.1:abc", "127.0.0.1:0", "127.0.0.1:65536", "::1:8080",
		"[::1]8080",
	} {
		t.Run(listenAddr, func(t *testing.T) {
			raw := validRaw()
			raw[httpListenAddr] = listenAddr
			if _, err := parse(raw); err == nil {
				t.Errorf("parse() error = nil, want error for listen address %q", listenAddr)
			}
		})
	}
}

func TestParseRejectsInvalidWOLDestinations(t *testing.T) {
	for _, destination := range []string{
		"192.168.1.0/24", "host.example", "::1", "192.168.1", "192.168.1.1.1",
		"192.168.1.256", "192.168.001.255", "192.168.-1.1",
	} {
		t.Run(destination, func(t *testing.T) {
			raw := validRaw()
			raw[wolDestination] = destination
			if _, err := parse(raw); err == nil {
				t.Errorf("parse() error = nil, want error for WOL destination %q", destination)
			}
		})
	}
}

func TestParseRejectsInvalidWOLPorts(t *testing.T) {
	for _, port := range []string{"", " ", " 9", "9 ", "nine", "0", "-1", "65536", "9.0"} {
		t.Run(port, func(t *testing.T) {
			raw := validRaw()
			raw[wolPort] = port
			if _, err := parse(raw); err == nil {
				t.Errorf("parse() error = nil, want error for WOL port %q", port)
			}
		})
	}
}

func TestParseRejectsInvalidMACAddresses(t *testing.T) {
	for _, mac := range []string{
		"aabbccddeeff", "aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:ff:00", "aa-bb:cc-dd-ee-ff",
		"aabb.ccdd.eef", "aabb.ccdd.eeff.00", "gg:bb:cc:dd:ee:ff",
	} {
		t.Run(mac, func(t *testing.T) {
			raw := validRaw()
			raw[wolMAC] = mac
			if _, err := parse(raw); err == nil {
				t.Errorf("parse() error = nil, want error for MAC %q", mac)
			}
		})
	}
}
