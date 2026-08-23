// Package shutdown contains the application boundary for graceful shutdown.
package shutdown

import "errors"

// ErrNotImplemented marks the temporary shutdown implementation scaffold.
var ErrNotImplemented = errors.New("graceful shutdown is not implemented")

// Port is the semantic outbound boundary used to request graceful shutdown.
type Port interface {
	Shutdown() error
}

// UseCase is the application behavior for a shutdown command.
type UseCase struct {
	port Port
}

// New creates a shutdown use case using port.
func New(port Port) *UseCase {
	return &UseCase{port: port}
}

// Shutdown is a structural placeholder until graceful-shutdown behavior is
// implemented.
func (u *UseCase) Shutdown() error {
	return ErrNotImplemented
}
