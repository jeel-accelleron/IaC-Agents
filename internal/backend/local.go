package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

// LocalBackend implements StateBackend for local filesystem
type LocalBackend struct {
	config BackendConfig
}

// NewLocalBackend creates a new local filesystem backend
func NewLocalBackend(config BackendConfig) (*LocalBackend, error) {
	if config.Connection == "" {
		return nil, fmt.Errorf("file path (connection) is required for local backend")
	}

	// Check if file exists
	if _, err := os.Stat(config.Connection); os.IsNotExist(err) {
		return nil, ErrStateNotFound{
			Backend: "local",
			Path:    config.Connection,
		}
	}

	return &LocalBackend{
		config: config,
	}, nil
}

// FetchState retrieves the Terraform state file from local filesystem
func (b *LocalBackend) FetchState(ctx context.Context) (*stateparser.StateFile, error) {
	// Read the file
	stateJSON, err := os.ReadFile(b.config.Connection)
	if err != nil {
		return nil, ErrStateNotFound{
			Backend: "local",
			Path:    b.config.Connection,
		}
	}

	// Parse the state file
	stateFile, err := stateparser.ParseStateFile(stateJSON)
	if err != nil {
		return nil, ErrInvalidState{Err: err}
	}

	return stateFile, nil
}

// ListStateFiles lists all .tfstate files in the local directory
func (b *LocalBackend) ListStateFiles(ctx context.Context) ([]string, error) {
	var stateFiles []string
	
	// Get the directory from the connection path
	dir := filepath.Dir(b.config.Connection)
	
	// Walk the directory to find all .tfstate files
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() && strings.HasSuffix(path, ".tfstate") {
			stateFiles = append(stateFiles, path)
		}
		
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to list local state files: %w", err)
	}
	
	return stateFiles, nil
}

// Type returns the backend type
func (b *LocalBackend) Type() string {
	return "local"
}
