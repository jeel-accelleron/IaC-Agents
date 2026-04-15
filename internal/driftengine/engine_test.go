package driftengine

import (
	"testing"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/cloudquery"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

func TestDetectUnmanaged(t *testing.T) {
	// State has 1 resource
	stateFile := &stateparser.StateFile{
		Resources: []stateparser.StateResource{
			{
				Type:    "azurerm_storage_account",
				Name:    "managed",
				AzureID: "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stmanaged",
			},
		},
	}

	// Azure has 2 resources (1 managed, 1 unmanaged)
	cloudResources := []cloudquery.CloudResource{
		{
			ID:            "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stmanaged",
			Type:          "Microsoft.Storage/storageAccounts",
			Name:          "stmanaged",
			ResourceGroup: "rg-prod",
		},
		{
			ID:            "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stunmanaged",
			Type:          "Microsoft.Storage/storageAccounts",
			Name:          "stunmanaged",
			ResourceGroup: "rg-prod",
		},
	}

	engine := NewDriftEngine(stateFile, cloudResources)
	findings := engine.DetectUnmanaged()

	if len(findings) != 1 {
		t.Errorf("DetectUnmanaged() found %d unmanaged resources, want 1", len(findings))
		return
	}

	if findings[0].Type != DriftTypeUnmanaged {
		t.Errorf("Finding type = %s, want %s", findings[0].Type, DriftTypeUnmanaged)
	}

	if findings[0].Severity != SeverityHigh {
		t.Errorf("Finding severity = %s, want %s", findings[0].Severity, SeverityHigh)
	}

	if findings[0].ResourceName != "stunmanaged" {
		t.Errorf("Finding resource name = %s, want stunmanaged", findings[0].ResourceName)
	}
}

func TestDetectOrphaned(t *testing.T) {
	// State has 2 resources
	stateFile := &stateparser.StateFile{
		Resources: []stateparser.StateResource{
			{
				Type:    "azurerm_storage_account",
				Name:    "exists",
				AzureID: "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stexists",
			},
			{
				Type:    "azurerm_storage_account",
				Name:    "deleted",
				AzureID: "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stdeleted",
			},
		},
	}

	// Azure has only 1 resource
	cloudResources := []cloudquery.CloudResource{
		{
			ID:            "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stexists",
			Type:          "Microsoft.Storage/storageAccounts",
			Name:          "stexists",
			ResourceGroup: "rg-prod",
		},
	}

	engine := NewDriftEngine(stateFile, cloudResources)
	findings := engine.DetectOrphaned()

	if len(findings) != 1 {
		t.Errorf("DetectOrphaned() found %d orphaned resources, want 1", len(findings))
		return
	}

	if findings[0].Type != DriftTypeOrphaned {
		t.Errorf("Finding type = %s, want %s", findings[0].Type, DriftTypeOrphaned)
	}

	if findings[0].Severity != SeverityMedium {
		t.Errorf("Finding severity = %s, want %s", findings[0].Severity, SeverityMedium)
	}

	if findings[0].StateName != "azurerm_storage_account.deleted" {
		t.Errorf("Finding state name = %s, want azurerm_storage_account.deleted", findings[0].StateName)
	}
}

func TestDetectAll(t *testing.T) {
	// State: managed, orphaned
	stateFile := &stateparser.StateFile{
		Resources: []stateparser.StateResource{
			{
				Type:    "azurerm_storage_account",
				Name:    "managed",
				AzureID: "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stmanaged",
			},
			{
				Type:    "azurerm_storage_account",
				Name:    "orphaned",
				AzureID: "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/storphaned",
			},
		},
	}

	// Azure: managed, unmanaged
	cloudResources := []cloudquery.CloudResource{
		{
			ID:            "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stmanaged",
			Type:          "Microsoft.Storage/storageAccounts",
			Name:          "stmanaged",
			ResourceGroup: "rg-prod",
		},
		{
			ID:            "/subscriptions/sub-123/resourceGroups/rg-prod/providers/Microsoft.Storage/storageAccounts/stunmanaged",
			Type:          "Microsoft.Storage/storageAccounts",
			Name:          "stunmanaged",
			ResourceGroup: "rg-prod",
		},
	}

	engine := NewDriftEngine(stateFile, cloudResources)
	findings, summary := engine.DetectAll()

	// Should find: 1 unmanaged, 1 orphaned
	if summary.UnmanagedCount != 1 {
		t.Errorf("Summary unmanaged count = %d, want 1", summary.UnmanagedCount)
	}

	if summary.OrphanedCount != 1 {
		t.Errorf("Summary orphaned count = %d, want 1", summary.OrphanedCount)
	}

	if summary.TotalFindings != 2 {
		t.Errorf("Summary total findings = %d, want 2", summary.TotalFindings)
	}

	if len(findings) != 2 {
		t.Errorf("Findings count = %d, want 2", len(findings))
	}
}

func TestFilterFindings(t *testing.T) {
	findings := []DriftFinding{
		{Type: DriftTypeUnmanaged},
		{Type: DriftTypeOrphaned},
		{Type: DriftTypeConfig},
	}

	tests := []struct {
		name       string
		driftTypes []DriftType
		wantCount  int
	}{
		{
			name:       "filter unmanaged only",
			driftTypes: []DriftType{DriftTypeUnmanaged},
			wantCount:  1,
		},
		{
			name:       "filter multiple types",
			driftTypes: []DriftType{DriftTypeUnmanaged, DriftTypeOrphaned},
			wantCount:  2,
		},
		{
			name:       "no filter",
			driftTypes: []DriftType{},
			wantCount:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := FilterFindings(findings, tt.driftTypes)
			if len(filtered) != tt.wantCount {
				t.Errorf("FilterFindings() count = %d, want %d", len(filtered), tt.wantCount)
			}
		})
	}
}
