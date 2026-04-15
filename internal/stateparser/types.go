package stateparser

// StateFile represents a parsed Terraform state file
type StateFile struct {
	Version       int             `json:"version"`
	TerraformVersion string       `json:"terraform_version"`
	Serial        int             `json:"serial"`
	Lineage       string          `json:"lineage"`
	Resources     []StateResource `json:"resources"`
}

// StateResource represents a single resource from the Terraform state
type StateResource struct {
	Type          string                 `json:"type"`           // e.g., "azurerm_virtual_machine"
	Name          string                 `json:"name"`           // Terraform resource name
	Provider      string                 `json:"provider"`       // Provider name
	Mode          string                 `json:"mode"`           // "managed" or "data"
	AzureID       string                 `json:"azure_id"`       // Full Azure resource ID
	ResourceGroup string                 `json:"resource_group"` // Extracted from ID
	Location      string                 `json:"location"`       // Azure region
	Attributes    map[string]interface{} `json:"attributes"`     // All resource properties
}

// RawStateFile represents the raw JSON structure of a Terraform state file
type RawStateFile struct {
	Version          int                      `json:"version"`
	TerraformVersion string                   `json:"terraform_version"`
	Serial           int                      `json:"serial"`
	Lineage          string                   `json:"lineage"`
	Resources        []RawStateResource       `json:"resources"`
	Outputs          map[string]interface{}   `json:"outputs,omitempty"`
}

// RawStateResource represents a resource in the raw state JSON
type RawStateResource struct {
	Mode      string                   `json:"mode"`
	Type      string                   `json:"type"`
	Name      string                   `json:"name"`
	Provider  string                   `json:"provider"`
	Instances []RawStateResourceInstance `json:"instances"`
}

// RawStateResourceInstance represents an instance of a resource
type RawStateResourceInstance struct {
	SchemaVersion int                    `json:"schema_version"`
	Attributes    map[string]interface{} `json:"attributes"`
	Dependencies  []string               `json:"dependencies,omitempty"`
}
