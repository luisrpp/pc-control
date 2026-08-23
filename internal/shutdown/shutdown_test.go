package shutdown_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/luisrpp/pc-control/internal/shutdown"
)

type recordingPort struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (p *recordingPort) Shutdown() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.err
}

func (p *recordingPort) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestShutdownAttemptsOneOperationAndReportsInitiation(t *testing.T) {
	port := &recordingPort{}
	useCase := shutdown.New(port)

	if err := useCase.Shutdown(); err != nil {
		t.Errorf("Shutdown() error = %v, want nil after remote initiation succeeds", err)
	}
	if got := port.Calls(); got != 1 {
		t.Errorf("shutdown port calls = %d, want 1", got)
	}
}

func TestShutdownReportsFailureAfterOneOperationWithoutRetry(t *testing.T) {
	port := &recordingPort{err: errors.New("remote shutdown failed")}
	useCase := shutdown.New(port)

	if err := useCase.Shutdown(); err == nil {
		t.Error("Shutdown() error = nil, want failure when the shutdown port fails")
	}
	if got := port.Calls(); got != 1 {
		t.Errorf("shutdown port calls = %d, want 1; failed operations must not be retried", got)
	}
}

func TestShutdownTreatsDuplicateCommandsAsIndependentOperations(t *testing.T) {
	port := &recordingPort{}
	useCase := shutdown.New(port)

	for i := 0; i < 3; i++ {
		if err := useCase.Shutdown(); err != nil {
			t.Errorf("Shutdown() call %d error = %v, want nil", i+1, err)
		}
	}
	if got := port.Calls(); got != 3 {
		t.Errorf("shutdown port calls = %d, want 3 independent duplicate-command operations", got)
	}
}

func TestShutdownMakesOneOperationForEachConcurrentCommand(t *testing.T) {
	const commands = 16

	port := &recordingPort{}
	useCase := shutdown.New(port)
	start := make(chan struct{})
	var calls sync.WaitGroup
	calls.Add(commands)

	for i := 0; i < commands; i++ {
		go func() {
			defer calls.Done()
			<-start
			if err := useCase.Shutdown(); err != nil {
				t.Errorf("Shutdown() error = %v, want nil", err)
			}
		}()
	}

	close(start)
	calls.Wait()

	if got := port.Calls(); got != commands {
		t.Errorf("shutdown port calls = %d, want %d", got, commands)
	}
}
