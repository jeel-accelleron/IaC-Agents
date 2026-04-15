package stateparser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	// azureResourceIDPattern matches Azure resource IDs
	azureResourceIDPattern = regexp.MustCompile(`^/subscriptions/[^/]+/resourceGroups/([^/]+)/providers/[^/]+/[^/]+/(.+)$`)
)

// ParseStateFile parses a Terraform state file JSON and extracts resources
func ParseStateFile(stateJSON []byte) (*StateFile, error) {
	var rawState RawStateFile
	if err := json.Unmarshal(stateJSON, &rawState); err != nil {
		return nil, fmt.Errorf("failed to parse state JSON: %w", err)
	}

	// Validate state version
	if rawState.Version < 4 {
		return nil, fmt.Errorf("unsupported state version %d (minimum supported: 4)", rawState.Version)
	}

	stateFile := &StateFile{
		Version:          rawState.Version,
		TerraformVersion: rawState.TerraformVersion,
		Serial:           rawState.Serial,
		Lineage:          rawState.Lineage,
		Resources:        make([]StateResource, 0),
	}

	// Extract resources
	for _, rawResource := range rawState.Resources {
		// Only process managed resources (not data sources)
		if rawResource.Mode != "managed" {
			continue
		}

		// Process each instance of the resource
		for _, instance := range rawResource.Instances {
			resource := StateResource{
				Type:       rawResource.Type,
				Name:       rawResource.Name,
				Provider:   rawResource.Provider,
				Mode:       rawResource.Mode,
				Attributes: instance.Attributes,
			}

			// Extract Azure-specific metadata
			if strings.HasPrefix(rawResource.Type, "azurerm_") {
				extractAzureMetadata(&resource)
			}

			stateFile.Resources = append(stateFile.Resources, resource)
		}
	}

	return stateFile, nil
}

// extractAzureMetadata extracts Azure-specific fields from resource attributes
func extractAzureMetadata(resource *StateResource) {
	attrs := resource.Attributes

	// Extract Azure resource ID
	if id, ok := attrs["id"].(string); ok {
		resource.AzureID = id
		
		// Extract resource group from ID
		if matches := azureResourceIDPattern.FindStringSubmatch(id); len(matches) > 1 {
			resource.ResourceGroup = matches[1]
		}
	}

	// Extract location/region
	if location, ok := attrs["location"].(string); ok {
		resource.Location = location
	}

	// Also check for resource_group_name attribute
	if rgName, ok := attrs["resource_group_name"].(string); ok && resource.ResourceGroup == "" {
		resource.ResourceGroup = rgName
	}
}

// GetResourceByID finds a resource in the state by its Azure resource ID
func (sf *StateFile) GetResourceByID(azureID string) *StateResource {
	normalizedID := normalizeAzureResourceID(azureID)
	
	for i := range sf.Resources {
		if normalizeAzureResourceID(sf.Resources[i].AzureID) == normalizedID {
			return &sf.Resources[i]
		}
	}
	
	return nil
}

// GetResourcesByType returns all resources of a specific type
func (sf *StateFile) GetResourcesByType(resourceType string) []StateResource {
	var resources []StateResource
	
	for _, resource := range sf.Resources {
		if resource.Type == resourceType {
			resources = append(resources, resource)
		}
	}
	
	return resources
}

// GetResourcesByResourceGroup returns all resources in a specific resource group
func (sf *StateFile) GetResourcesByResourceGroup(resourceGroup string) []StateResource {
	var resources []StateResource
	normalizedRG := strings.ToLower(resourceGroup)
	
	for _, resource := range sf.Resources {
		if strings.ToLower(resource.ResourceGroup) == normalizedRG {
			resources = append(resources, resource)
		}
	}
	
	return resources
}

// normalizeAzureResourceID normalizes Azure resource IDs for comparison
// Azure resource IDs are case-insensitive
func normalizeAzureResourceID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// GetAllAzureResourceIDs returns a set of all Azure resource IDs in the state
func (sf *StateFile) GetAllAzureResourceIDs() map[string]bool {
	ids := make(map[string]bool)
	
	for _, resource := range sf.Resources {
		if resource.AzureID != "" {
			ids[normalizeAzureResourceID(resource.AzureID)] = true
		}
	}
	
	return ids
}
