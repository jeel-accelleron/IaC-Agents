package stateparser

import (
	"testing"
)

func TestParseStateFile(t *testing.T) {
	tests := []struct {
		name        string
		stateJSON   string
		wantErr     bool
		wantVersion int
		wantCount   int
	}{
		{
			name: "valid state v4",
			stateJSON: `{
				"version": 4,
				"terraform_version": "1.5.0",
				"serial": 1,
				"lineage": "test-lineage",
				"resources": [
					{
						"mode": "managed",
						"type": "azurerm_storage_account",
						"name": "example",
						"provider": "provider[\"registry.terraform.io/hashicorp/azurerm\"]",
						"instances": [
							{
								"schema_version": 3,
								"attributes": {
									"id": "/subscriptions/sub-123/resourceGroups/rg-test/providers/Microsoft.Storage/storageAccounts/sttest",
									"name": "sttest",
									"location": "eastus",
									"resource_group_name": "rg-test",
									"tags": {
										"Environment": "test"
									}
								}
							}
						]
					}
				]
			}`,
			wantErr:     false,
			wantVersion: 4,
			wantCount:   1,
		},
		{
			name: "unsupported state version",
			stateJSON: `{
				"version": 3,
				"resources": []
			}`,
			wantErr: true,
		},
		{
			name:      "invalid JSON",
			stateJSON: `{invalid json`,
			wantErr:   true,
		},
		{
			name: "data sources excluded",
			stateJSON: `{
				"version": 4,
				"terraform_version": "1.5.0",
				"serial": 1,
				"lineage": "test",
				"resources": [
					{
						"mode": "data",
						"type": "azurerm_resource_group",
						"name": "example",
						"provider": "provider[\"registry.terraform.io/hashicorp/azurerm\"]",
						"instances": [{"schema_version": 0, "attributes": {"id": "/test"}}]
					}
				]
			}`,
			wantErr:     false,
			wantVersion: 4,
			wantCount:   0, // Data sources should be excluded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateFile, err := ParseStateFile([]byte(tt.stateJSON))
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseStateFile() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("ParseStateFile() unexpected error: %v", err)
				return
			}

			if stateFile.Version != tt.wantVersion {
				t.Errorf("Version = %d, want %d", stateFile.Version, tt.wantVersion)
			}

			if len(stateFile.Resources) != tt.wantCount {
				t.Errorf("Resource count = %d, want %d", len(stateFile.Resources), tt.wantCount)
			}
		})
	}
}

func TestGetResourceByID(t *testing.T) {
	stateJSON := `{
		"version": 4,
		"terraform_version": "1.5.0",
		"serial": 1,
		"lineage": "test",
		"resources": [
			{
				"mode": "managed",
				"type": "azurerm_virtual_machine",
				"name": "web01",
				"provider": "provider[\"registry.terraform.io/hashicorp/azurerm\"]",
				"instances": [{
					"schema_version": 0,
					"attributes": {
						"id": "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-web01",
						"name": "vm-web01",
						"location": "eastus"
					}
				}]
			}
		]
	}`

	stateFile, err := ParseStateFile([]byte(stateJSON))
	if err != nil {
		t.Fatalf("ParseStateFile() error: %v", err)
	}

	tests := []struct {
		name     string
		azureID  string
		wantName string
		wantNil  bool
	}{
		{
			name:     "exact match",
			azureID:  "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-web01",
			wantName: "web01",
			wantNil:  false,
		},
		{
			name:     "case insensitive match",
			azureID:  "/SUBSCRIPTIONS/SUB-123/RESOURCEGROUPS/RG-PROD/PROVIDERS/MICROSOFT.COMPUTE/VIRTUALMACHINES/VM-WEB01",
			wantName: "web01",
			wantNil:  false,
		},
		{
			name:    "not found",
			azureID: "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm-notfound",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := stateFile.GetResourceByID(tt.azureID)
			
			if tt.wantNil {
				if resource != nil {
					t.Errorf("GetResourceByID() = %v, want nil", resource)
				}
				return
			}

			if resource == nil {
				t.Errorf("GetResourceByID() = nil, want resource")
				return
			}

			if resource.Name != tt.wantName {
				t.Errorf("Resource.Name = %s, want %s", resource.Name, tt.wantName)
			}
		})
	}
}

func TestGetAllAzureResourceIDs(t *testing.T) {
	stateJSON := `{
		"version": 4,
		"terraform_version": "1.5.0",
		"serial": 1,
		"lineage": "test",
		"resources": [
			{
				"mode": "managed",
				"type": "azurerm_storage_account",
				"name": "st1",
				"provider": "provider[\"registry.terraform.io/hashicorp/azurerm\"]",
				"instances": [{
					"schema_version": 0,
					"attributes": {
						"id": "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/st1"
					}
				}]
			},
			{
				"mode": "managed",
				"type": "azurerm_storage_account",
				"name": "st2",
				"provider": "provider[\"registry.terraform.io/hashicorp/azurerm\"]",
				"instances": [{
					"schema_version": 0,
					"attributes": {
						"id": "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/st2"
					}
				}]
			}
		]
	}`

	stateFile, err := ParseStateFile([]byte(stateJSON))
	if err != nil {
		t.Fatalf("ParseStateFile() error: %v", err)
	}

	ids := stateFile.GetAllAzureResourceIDs()

	if len(ids) != 2 {
		t.Errorf("GetAllAzureResourceIDs() count = %d, want 2", len(ids))
	}

	// IDs should be normalized (lowercase)
	expectedID := "/subscriptions/sub-123/resourcegroups/rg-prod/providers/microsoft.storage/storageaccounts/st1"
	if !ids[expectedID] {
		t.Errorf("GetAllAzureResourceIDs() missing normalized ID: %s", expectedID)
	}
}
