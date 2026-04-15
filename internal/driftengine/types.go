package driftengine

import (
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/cloudquery"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

// DriftType represents the type of drift detected
type DriftType string

const (
	DriftTypeUnmanaged DriftType = "unmanaged" // Resource in Azure but not in state
	DriftTypeOrphaned  DriftType = "orphaned"  // Resource in state but not in Azure
	DriftTypeConfig    DriftType = "config"    // Resource exists in both but properties differ
)

// Severity represents the severity level of a drift finding
type Severity string

const (
	SeverityHigh   Severity = "HIGH"
	SeverityMedium Severity = "MEDIUM"
	SeverityLow    Severity = "LOW"
)

// DriftFinding represents a detected drift
type DriftFinding struct {
	Type              DriftType              `json:"type"`
	Severity          Severity               `json:"severity"`
	ResourceID        string                 `json:"resource_id"`
	ResourceType      string                 `json:"resource_type"`
	ResourceName      string                 `json:"resource_name"`
	ResourceGroup     string                 `json:"resource_group"`
	Location          string                 `json:"location,omitempty"`
	StateName         string                 `json:"state_name,omitempty"`      // Terraform resource name
	PropertyDiffs     []PropertyDiff         `json:"property_diffs,omitempty"`  // For config drift
	Recommendation    string                 `json:"recommendation"`
	Details           string                 `json:"details,omitempty"`
}

// PropertyDiff represents a difference in a specific property
type PropertyDiff struct {
	Path        string      `json:"path"`          // Property path (e.g., "tags.Environment")
	StateValue  interface{} `json:"state_value"`   // Value from Terraform state
	AzureValue  interface{} `json:"azure_value"`   // Value from Azure
	Severity    Severity    `json:"severity"`      // Severity of this specific diff
	Category    string      `json:"category"`      // Category (security, networking, config, tags)
}

// DriftSummary provides a summary of all drift findings
type DriftSummary struct {
	TotalFindings    int                    `json:"total_findings"`
	UnmanagedCount   int                    `json:"unmanaged_count"`
	OrphanedCount    int                    `json:"orphaned_count"`
	ConfigDriftCount int                    `json:"config_drift_count"`
	BySeverity       map[Severity]int       `json:"by_severity"`
	ByType           map[DriftType]int      `json:"by_type"`
	StateResources   int                    `json:"state_resources"`
	AzureResources   int                    `json:"azure_resources"`
}

// DriftEngine compares Terraform state with Azure resources
type DriftEngine struct {
	StateFile      *stateparser.StateFile
	CloudResources []cloudquery.CloudResource
	comparator     *PropertyComparator
}

// NewDriftEngine creates a new drift detection engine
func NewDriftEngine(
	stateFile *stateparser.StateFile,
	cloudResources []cloudquery.CloudResource,
) *DriftEngine {
	return &DriftEngine{
		StateFile:      stateFile,
		CloudResources: cloudResources,
		comparator:     NewPropertyComparator(),
	}
}
