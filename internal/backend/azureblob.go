package backend

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

// AzureBlobBackend implements StateBackend for Azure Blob Storage
type AzureBlobBackend struct {
	config BackendConfig
	client *azblob.Client
}

// NewAzureBlobBackend creates a new Azure Blob Storage backend
func NewAzureBlobBackend(config BackendConfig) (*AzureBlobBackend, error) {
	if config.Connection == "" {
		return nil, fmt.Errorf("connection string is required for Azure Blob backend")
	}
	
	if config.Container == "" {
		return nil, fmt.Errorf("container name is required for Azure Blob backend")
	}
	
	if config.Key == "" {
		return nil, fmt.Errorf("blob key/path is required for Azure Blob backend")
	}

	var client *azblob.Client
	var err error

	// Detect if Connection is a URL or a connection string
	isURL := strings.HasPrefix(config.Connection, "https://") || strings.HasPrefix(config.Connection, "http://")

	if isURL {
		// Use DefaultAzureCredential for URL-based authentication
		cred, credErr := azidentity.NewDefaultAzureCredential(nil)
		if credErr != nil {
			return nil, fmt.Errorf("failed to create Azure credential: %w", credErr)
		}
		client, err = azblob.NewClient(config.Connection, cred, nil)
	} else {
		// Use connection string
		client, err = azblob.NewClientFromConnectionString(config.Connection, nil)
	}

	if err != nil {
		return nil, ErrBackendConnection{Backend: "azureblob", Err: err}
	}

	return &AzureBlobBackend{
		config: config,
		client: client,
	}, nil
}

// FetchState retrieves the Terraform state file from Azure Blob Storage
func (b *AzureBlobBackend) FetchState(ctx context.Context) (*stateparser.StateFile, error) {
	// Download the blob
	resp, err := b.client.DownloadStream(ctx, b.config.Container, b.config.Key, nil)
	if err != nil {
		return nil, ErrStateNotFound{
			Backend: "azureblob",
			Path:    fmt.Sprintf("%s/%s", b.config.Container, b.config.Key),
		}
	}
	defer resp.Body.Close()

	// Read the blob content
	stateJSON, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob content: %w", err)
	}

	// Parse the state file
	stateFile, err := stateparser.ParseStateFile(stateJSON)
	if err != nil {
		return nil, ErrInvalidState{Err: err}
	}

	return stateFile, nil
}

// ListStateFiles lists all .tfstate files in the container
func (b *AzureBlobBackend) ListStateFiles(ctx context.Context) ([]string, error) {
	var stateFiles []string
	
	// List blobs in the container
	pager := b.client.NewListBlobsFlatPager(b.config.Container, nil)
	
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list blobs: %w", err)
		}
		
		for _, blob := range page.Segment.BlobItems {
			if blob.Name != nil {
				// Filter for .tfstate files only
				if strings.HasSuffix(*blob.Name, ".tfstate") {
					stateFiles = append(stateFiles, *blob.Name)
				}
			}
		}
	}
	
	return stateFiles, nil
}

// Type returns the backend type
func (b *AzureBlobBackend) Type() string {
	return "azureblob"
}
