// Package shutdown contains the application boundary for graceful shutdown.
package shutdown

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

// Shutdown performs one graceful-shutdown operation.
func (u *UseCase) Shutdown() error {
	return u.port.Shutdown()
}
