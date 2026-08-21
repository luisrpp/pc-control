// Package wake contains the application use case for requesting a wake.
package wake

// Sender is the outbound boundary used by the wake use case.
//
// Send deliberately does not accept a request context: after a wake command
// has been accepted, an HTTP client cancellation must not cancel the attempt.
type Sender interface {
	Send() error
}

// UseCase is the application behavior for a wake command.
type UseCase struct {
	sender Sender
}

// New creates a wake use case using sender.
func New(sender Sender) *UseCase {
	return &UseCase{sender: sender}
}

// Wake performs a wake command.
func (u *UseCase) Wake() error {
	return u.sender.Send()
}
