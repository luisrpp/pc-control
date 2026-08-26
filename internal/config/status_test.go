package config

import (
	"testing"
	"time"
)

func TestParseShutdownDefaultsStatusProbeTimeoutOnlyWhenAbsent(t *testing.T) {
	raw := validShutdownRaw()

	got, err := parseShutdown(raw)
	if err != nil {
		t.Fatalf("parseShutdown() error = %v, want nil", err)
	}
	if got.StatusProbeTimeout != time.Second {
		t.Errorf("StatusProbeTimeout = %v, want 1s when %s is absent", got.StatusProbeTimeout, envStatusProbeTimeout)
	}
}

func TestParseShutdownAcceptsPositiveStatusProbeTimeout(t *testing.T) {
	raw := validShutdownRaw()
	raw[envStatusProbeTimeout] = "1500ms"

	got, err := parseShutdown(raw)
	if err != nil {
		t.Fatalf("parseShutdown() error = %v, want nil", err)
	}
	if got.StatusProbeTimeout != 1500*time.Millisecond {
		t.Errorf("StatusProbeTimeout = %v, want 1500ms", got.StatusProbeTimeout)
	}
}

func TestParseShutdownRejectsInvalidStatusProbeTimeout(t *testing.T) {
	for _, value := range []string{"", " ", " 1s", "1s ", "not-a-duration", "0", "0s", "-1s"} {
		t.Run(value, func(t *testing.T) {
			raw := validShutdownRaw()
			raw[envStatusProbeTimeout] = value
			if _, err := parseShutdown(raw); err == nil {
				t.Errorf("parseShutdown() error = nil, want error for %s=%q", envStatusProbeTimeout, value)
			}
		})
	}
}
