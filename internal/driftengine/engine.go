package driftengine

import (
	"fmt"
	"strings"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/cloudquery"
)

// DetectAll runs all drift detection types
func (e *DriftEngine) DetectAll() ([]DriftFinding, *DriftSummary) {
	var allFindings []DriftFinding

	// Detect unmanaged resources
	unmanaged := e.DetectUnmanaged()
	allFindings = append(allFindings, unmanaged...)

	// Detect orphaned state entries
	orphaned := e.DetectOrphaned()
	allFindings = append(allFindings, orphaned...)

	// Detect configuration drift
	configDrift := e.DetectConfigDrift()
	allFindings = append(allFindings, configDrift...)

	// Generate summary
	summary := e.generateSummary(allFindings)

	return allFindings, summary
}

// DetectUnmanaged finds resources that exist in Azure but not in Terraform state
func (e *DriftEngine) DetectUnmanaged() []DriftFinding {
	var findings []DriftFinding

	// Build a set of all Azure resource IDs from state
	stateIDs := e.StateFile.GetAllAzureResourceIDs()

	// Check each Azure resource
	for _, cloudResource := range e.CloudResources {
		normalizedID := strings.ToLower(cloudResource.ID)

		// If not in state, it's unmanaged
		if !stateIDs[normalizedID] {
			finding := DriftFinding{
				Type:           DriftTypeUnmanaged,
				Severity:       SeverityHigh,
				ResourceID:     cloudResource.ID,
				ResourceType:   cloudResource.Type,
				ResourceName:   cloudResource.Name,
				ResourceGroup:  cloudResource.ResourceGroup,
				Location:       cloudResource.Location,
				Recommendation: "Review this resource. Consider importing it into Terraform using 'terraform import' or removing it if it's no longer needed.",
				Details:        fmt.Sprintf("Resource created outside Terraform (not found in state file). This may indicate shadow IT or manual provisioning."),
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

// DetectOrphaned finds resources that exist in Terraform state but not in Azure
func (e *DriftEngine) DetectOrphaned() []DriftFinding {
	var findings []DriftFinding

	// Build a map of Azure resources by ID
	cloudResourceMap := cloudquery.GetResourcesByIDs(e.CloudResources)

	// Check each state resource
	for _, stateResource := range e.StateFile.Resources {
		if stateResource.AzureID == "" {
			continue // Skip resources without Azure IDs
		}

		normalizedID := strings.ToLower(stateResource.AzureID)

		// If not in Azure, it's orphaned
		if _, exists := cloudResourceMap[normalizedID]; !exists {
			finding := DriftFinding{
				Type:           DriftTypeOrphaned,
				Severity:       SeverityMedium,
				ResourceID:     stateResource.AzureID,
				ResourceType:   stateResource.Type,
				ResourceName:   stateResource.Name,
				ResourceGroup:  stateResource.ResourceGroup,
				Location:       stateResource.Location,
				StateName:      fmt.Sprintf("%s.%s", stateResource.Type, stateResource.Name),
				Recommendation: "Remove this resource from Terraform state using 'terraform state rm' or recreate the resource if it was deleted accidentally.",
				Details:        "Resource exists in Terraform state but not in Azure. This may indicate manual deletion or a failed destroy operation.",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

// DetectConfigDrift finds resources that exist in both state and Azure but have different properties
func (e *DriftEngine) DetectConfigDrift() []DriftFinding {
	var findings []DriftFinding

	// Build a map of Azure resources by ID
	cloudResourceMap := cloudquery.GetResourcesByIDs(e.CloudResources)

	// Check each state resource
	for _, stateResource := range e.StateFile.Resources {
		if stateResource.AzureID == "" {
			continue // Skip resources without Azure IDs
		}

		normalizedID := strings.ToLower(stateResource.AzureID)

		// Check if resource exists in both
		if cloudResource, exists := cloudResourceMap[normalizedID]; exists {
			// Compare properties
			diffs := e.comparator.CompareProperties(stateResource, cloudResource)

			// If there are differences, create a finding
			if len(diffs) > 0 {
				// Determine overall severity based on most severe diff
				severity := SeverityLow
				for _, diff := range diffs {
					if diff.Severity == SeverityHigh {
						severity = SeverityHigh
						break
					} else if diff.Severity == SeverityMedium && severity != SeverityHigh {
						severity = SeverityMedium
					}
				}

				finding := DriftFinding{
					Type:           DriftTypeConfig,
					Severity:       severity,
					ResourceID:     stateResource.AzureID,
					ResourceType:   stateResource.Type,
					ResourceName:   cloudResource.Name,
					ResourceGroup:  cloudResource.ResourceGroup,
					Location:       cloudResource.Location,
					StateName:      fmt.Sprintf("%s.%s", stateResource.Type, stateResource.Name),
					PropertyDiffs:  diffs,
					Recommendation: "Run 'terraform plan' to see the drift, then either apply changes to reconcile or update your Terraform code to match the current Azure configuration.",
					Details:        fmt.Sprintf("Found %d property difference(s) between Terraform state and actual Azure configuration.", len(diffs)),
				}
				findings = append(findings, finding)
			}
		}
	}

	return findings
}

// generateSummary creates a summary of all drift findings
func (e *DriftEngine) generateSummary(findings []DriftFinding) *DriftSummary {
	summary := &DriftSummary{
		TotalFindings:  len(findings),
		BySeverity:     make(map[Severity]int),
		ByType:         make(map[DriftType]int),
		StateResources: len(e.StateFile.Resources),
		AzureResources: len(e.CloudResources),
	}

	for _, finding := range findings {
		summary.BySeverity[finding.Severity]++
		summary.ByType[finding.Type]++

		switch finding.Type {
		case DriftTypeUnmanaged:
			summary.UnmanagedCount++
		case DriftTypeOrphaned:
			summary.OrphanedCount++
		case DriftTypeConfig:
			summary.ConfigDriftCount++
		}
	}

	return summary
}

// FilterFindings filters findings based on drift types
func FilterFindings(findings []DriftFinding, driftTypes []DriftType) []DriftFinding {
	if len(driftTypes) == 0 {
		return findings // No filter, return all
	}

	typeMap := make(map[DriftType]bool)
	for _, dt := range driftTypes {
		typeMap[dt] = true
	}

	var filtered []DriftFinding
	for _, finding := range findings {
		if typeMap[finding.Type] {
			filtered = append(filtered, finding)
		}
	}

	return filtered
}
