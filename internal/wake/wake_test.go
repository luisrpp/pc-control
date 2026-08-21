package wake_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/luisrpp/pc-control/internal/wake"
)

type recordingSender struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (s *recordingSender) Send() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.err
}

func (s *recordingSender) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestWakeAttemptsOneSendAndReportsSuccess(t *testing.T) {
	sender := &recordingSender{}
	useCase := wake.New(sender)

	if err := useCase.Wake(); err != nil {
		t.Errorf("Wake() error = %v, want nil after a successful local send", err)
	}
	if got := sender.Calls(); got != 1 {
		t.Errorf("sender calls = %d, want 1", got)
	}
}

func TestWakeReportsFailureAfterOneAttemptWithoutRetry(t *testing.T) {
	sendErr := errors.New("local UDP send failed")
	sender := &recordingSender{err: sendErr}
	useCase := wake.New(sender)

	if err := useCase.Wake(); err == nil {
		t.Error("Wake() error = nil, want failure when the sender fails")
	}
	if got := sender.Calls(); got != 1 {
		t.Errorf("sender calls = %d, want 1; failed sends must not be retried", got)
	}
}

func TestWakeTreatsDuplicateCommandsAsIndependentAttempts(t *testing.T) {
	sender := &recordingSender{}
	useCase := wake.New(sender)

	for i := 0; i < 3; i++ {
		if err := useCase.Wake(); err != nil {
			t.Errorf("Wake() call %d error = %v, want nil", i+1, err)
		}
	}
	if got := sender.Calls(); got != 3 {
		t.Errorf("sender calls = %d, want 3 independent duplicate-command attempts", got)
	}
}

func TestWakeMakesOneAttemptForEachConcurrentCommand(t *testing.T) {
	const commands = 16

	sender := &recordingSender{}
	useCase := wake.New(sender)
	start := make(chan struct{})
	var calls sync.WaitGroup
	calls.Add(commands)

	for i := 0; i < commands; i++ {
		go func() {
			defer calls.Done()
			<-start
			if err := useCase.Wake(); err != nil {
				t.Errorf("Wake() error = %v, want nil", err)
			}
		}()
	}

	close(start)
	calls.Wait()

	if got := sender.Calls(); got != commands {
		t.Errorf("sender calls = %d, want %d", got, commands)
	}
}
