package backend

import "fmt"

// ErrUnsupportedBackend is returned when an unsupported backend type is requested
type ErrUnsupportedBackend struct {
	Type string
}

func (e ErrUnsupportedBackend) Error() string {
	return fmt.Sprintf("unsupported backend type: %s", e.Type)
}

// ErrBackendConnection is returned when there's a connection error to the backend
type ErrBackendConnection struct {
	Backend string
	Err     error
}

func (e ErrBackendConnection) Error() string {
	return fmt.Sprintf("backend connection error (%s): %v", e.Backend, e.Err)
}

func (e ErrBackendConnection) Unwrap() error {
	return e.Err
}

// ErrStateNotFound is returned when the state file doesn't exist
type ErrStateNotFound struct {
	Backend string
	Path    string
}

func (e ErrStateNotFound) Error() string {
	return fmt.Sprintf("state file not found in %s: %s", e.Backend, e.Path)
}

// ErrInvalidState is returned when the state file is invalid or corrupted
type ErrInvalidState struct {
	Err error
}

func (e ErrInvalidState) Error() string {
	return fmt.Sprintf("invalid state file: %v", e.Err)
}

func (e ErrInvalidState) Unwrap() error {
	return e.Err
}
