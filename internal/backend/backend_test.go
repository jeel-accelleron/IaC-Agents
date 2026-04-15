package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalBackend(t *testing.T) {
	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "terraform.tfstate")

	validState := `{
		"version": 4,
		"terraform_version": "1.5.0",
		"serial": 1,
		"lineage": "test",
		"resources": []
	}`

	if err := os.WriteFile(stateFile, []byte(validState), 0644); err != nil {
		t.Fatalf("Failed to create test state file: %v", err)
	}

	tests := []struct {
		name      string
		config    BackendConfig
		wantErr   bool
		errType   string
	}{
		{
			name: "valid local file",
			config: BackendConfig{
				Type:       "local",
				Connection: stateFile,
			},
			wantErr: false,
		},
		{
			name: "file not found",
			config: BackendConfig{
				Type:       "local",
				Connection: filepath.Join(tmpDir, "nonexistent.tfstate"),
			},
			wantErr: true,
			errType: "ErrStateNotFound",
		},
		{
			name: "missing path",
			config: BackendConfig{
				Type:       "local",
				Connection: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewLocalBackend(tt.config)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewLocalBackend() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("NewLocalBackend() unexpected error: %v", err)
				return
			}

			if backend.Type() != "local" {
				t.Errorf("Backend type = %s, want local", backend.Type())
			}

			// Try to fetch state
			ctx := context.Background()
			state, err := backend.FetchState(ctx)
			if err != nil {
				t.Errorf("FetchState() error: %v", err)
				return
			}

			if state.Version != 4 {
				t.Errorf("State version = %d, want 4", state.Version)
			}
		})
	}
}

func TestNewBackend(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "test.tfstate")
	os.WriteFile(stateFile, []byte(`{"version": 4, "resources": []}`), 0644)

	tests := []struct {
		name    string
		config  BackendConfig
		wantErr bool
	}{
		{
			name: "local backend",
			config: BackendConfig{
				Type:       "local",
				Connection: stateFile,
			},
			wantErr: false,
		},
		{
			name: "unsupported backend",
			config: BackendConfig{
				Type: "unsupported",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBackend(tt.config)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewBackend() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("NewBackend() unexpected error: %v", err)
			}
		})
	}
}
