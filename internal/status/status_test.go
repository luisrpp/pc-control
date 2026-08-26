package status_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/luisrpp/pc-control/internal/status"
)

type recordingProbe struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (p *recordingProbe) Probe() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.err
}

func (p *recordingProbe) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestStatusPerformsOneProbeAndReportsOnline(t *testing.T) {
	probe := &recordingProbe{}
	useCase := status.New(probe)

	if got := useCase.Status(); got != status.Online {
		t.Errorf("Status() = %q, want %q after a successful probe", got, status.Online)
	}
	if got := probe.Calls(); got != 1 {
		t.Errorf("probe calls = %d, want 1", got)
	}
}

func TestStatusReportsOfflineAfterOneFailedProbeWithoutRetry(t *testing.T) {
	probe := &recordingProbe{err: errors.New("controlled probe failure")}
	useCase := status.New(probe)

	if got := useCase.Status(); got != status.Offline {
		t.Errorf("Status() = %q, want %q after a failed probe", got, status.Offline)
	}
	if got := probe.Calls(); got != 1 {
		t.Errorf("probe calls = %d, want 1; failed probes must not be retried", got)
	}
}

func TestStatusTreatsDuplicateRequestsAsIndependentProbes(t *testing.T) {
	probe := &recordingProbe{}
	useCase := status.New(probe)

	for i := 0; i < 3; i++ {
		if got := useCase.Status(); got != status.Online {
			t.Errorf("Status() call %d = %q, want %q", i+1, got, status.Online)
		}
	}
	if got := probe.Calls(); got != 3 {
		t.Errorf("probe calls = %d, want 3 independent duplicate-request probes", got)
	}
}

func TestStatusPerformsOneProbeForEachConcurrentRequest(t *testing.T) {
	const requests = 16

	probe := &recordingProbe{}
	useCase := status.New(probe)
	start := make(chan struct{})
	var calls sync.WaitGroup
	calls.Add(requests)

	for i := 0; i < requests; i++ {
		go func() {
			defer calls.Done()
			<-start
			if got := useCase.Status(); got != status.Online {
				t.Errorf("Status() = %q, want %q", got, status.Online)
			}
		}()
	}

	close(start)
	calls.Wait()

	if got := probe.Calls(); got != requests {
		t.Errorf("probe calls = %d, want %d", got, requests)
	}
}
