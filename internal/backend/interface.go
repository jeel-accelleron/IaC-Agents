package backend

import (
	"context"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

// StateBackend is the interface for fetching Terraform state from various backends
type StateBackend interface {
	// FetchState retrieves the Terraform state file
	FetchState(ctx context.Context) (*stateparser.StateFile, error)
	
	// ListStateFiles lists all state files in the backend (for scan-all mode)
	ListStateFiles(ctx context.Context) ([]string, error)
	
	// Type returns the backend type (azureblob, s3, tfc, local)
	Type() string
}

// BackendConfig holds configuration for a state backend
type BackendConfig struct {
	Type       string `json:"type"`       // azureblob, s3, tfc, local
	Connection string `json:"connection"` // Connection string, path, or API endpoint
	Credential string `json:"credential"` // SAS token, access key, or API token
	Container  string `json:"container"`  // Container/bucket name (for blob/s3)
	Key        string `json:"key"`        // State file path within container/bucket
}

// NewBackend creates a new backend based on the configuration
func NewBackend(config BackendConfig) (StateBackend, error) {
	switch config.Type {
	case "azureblob":
		return NewAzureBlobBackend(config)
	case "s3":
		return NewS3Backend(config)
	case "local":
		return NewLocalBackend(config)
	default:
		return nil, ErrUnsupportedBackend{Type: config.Type}
	}
}
