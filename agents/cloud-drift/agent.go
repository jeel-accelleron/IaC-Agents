package clouddrift

import (
	"context"
	"fmt"
	"strings"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/backend"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/cloudquery"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/config"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/driftengine"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/protocol"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

// Agent implements cloud drift detection
type Agent struct {
	config *config.Config
}

// NewAgent creates a new cloud drift agent
func NewAgent(cfg *config.Config) *Agent {
	return &Agent{
		config: cfg,
	}
}

// ID returns the agent identifier
func (a *Agent) ID() string {
	return "cloud-drift"
}

// Metadata returns agent metadata
func (a *Agent) Metadata() protocol.AgentMetadata {
	return protocol.AgentMetadata{
		ID:          "cloud-drift",
		Name:        "Cloud Drift Detection",
		Description: "Detects infrastructure drift by comparing Terraform state with actual Azure resources",
		Version:     "1.0.0",
	}
}

// Capabilities returns agent capabilities
func (a *Agent) Capabilities() protocol.AgentCapabilities {
	return protocol.AgentCapabilities{
		Formats:       []protocol.SourceFormat{protocol.FormatTerraform},
		NeedsIaCInput: false, // Works with state files, not IaC code
	}
}

// Handle processes a cloud drift detection request
func (a *Agent) Handle(ctx context.Context, request protocol.AgentRequest, emitter protocol.Emitter) error {
	// Emit header
	emitter.SendMessage("## Cloud Drift Detection\n\n")

	// Get configuration from request or use default config
	backendConfig := a.getBackendConfig(request)
	scopeConfig := a.getScopeConfig(request)
	driftTypes := a.getDriftTypes(request)

	// Step 1: Fetch Terraform state
	emitter.SendMessage("**Step 1/4:** Fetching Terraform state file(s)...\n")
	
	stateBackend, err := backend.NewBackend(backendConfig)
	if err != nil {
		return a.emitError(emitter, "Failed to initialize state backend", err)
	}

	var stateFile *stateparser.StateFile
	
	// Check if scan-all mode is enabled
	if a.config.TFStateScanAll {
		stateFile, err = a.fetchAllStateFiles(ctx, stateBackend, emitter)
	} else {
		stateFile, err = stateBackend.FetchState(ctx)
	}
	
	if err != nil {
		return a.emitError(emitter, "Failed to fetch Terraform state", err)
	}

	emitter.SendMessage(fmt.Sprintf("✓ State file(s) loaded: %d resources found\n\n", len(stateFile.Resources)))

	// Step 2: Query Azure resources
	emitter.SendMessage("**Step 2/4:** Querying Azure resources...\n")
	
	azureClient, err := cloudquery.NewAzureClient(ctx)
	if err != nil {
		return a.emitError(emitter, "Failed to initialize Azure client", err)
	}

	cloudResources, err := azureClient.QueryResources(ctx, scopeConfig)
	if err != nil {
		return a.emitError(emitter, "Failed to query Azure resources", err)
	}

	emitter.SendMessage(fmt.Sprintf("✓ Azure resources found: %d\n\n", len(cloudResources)))

	// Step 3: Detect drift
	emitter.SendMessage("**Step 3/4:** Analyzing drift...\n")
	
	engine := driftengine.NewDriftEngine(stateFile, cloudResources)
	findings, summary := engine.DetectAll()

	// Filter findings by requested drift types
	if len(driftTypes) > 0 {
		findings = driftengine.FilterFindings(findings, driftTypes)
	}

	emitter.SendMessage(fmt.Sprintf("✓ Analysis complete: %d drift finding(s) detected\n\n", len(findings)))

	// Step 4: Emit results
	emitter.SendMessage("**Step 4/4:** Generating report...\n\n")
	
	a.emitResults(emitter, findings, summary, scopeConfig)

	return nil
}

// getBackendConfig extracts backend configuration from request or config
func (a *Agent) getBackendConfig(request protocol.AgentRequest) backend.BackendConfig {
	// Try to extract from agent-specific input
	// For now, use config or environment-based defaults
	return backend.BackendConfig{
		Type:       a.config.TFStateBackendType,
		Connection: a.config.TFStateBackendConnection,
		Credential: a.config.TFStateBackendCredential,
		Container:  a.config.TFStateContainer,
		Key:        a.config.TFStateKey,
	}
}

// getScopeConfig extracts scope configuration from request or config
func (a *Agent) getScopeConfig(request protocol.AgentRequest) cloudquery.ScopeConfig {
	return cloudquery.ScopeConfig{
		SubscriptionID: a.config.AzureSubscriptionID,
		ResourceGroups: a.config.DriftScopeResourceGroups,
		Tags:           a.config.DriftScopeTags,
		ResourceTypes:  a.config.DriftScopeResourceTypes,
	}
}

// getDriftTypes extracts drift types to detect from request or config
func (a *Agent) getDriftTypes(request protocol.AgentRequest) []driftengine.DriftType {
	// Parse from config
	var types []driftengine.DriftType
	for _, t := range a.config.DriftTypes {
		switch strings.ToLower(t) {
		case "unmanaged":
			types = append(types, driftengine.DriftTypeUnmanaged)
		case "orphaned":
			types = append(types, driftengine.DriftTypeOrphaned)
		case "config":
			types = append(types, driftengine.DriftTypeConfig)
		}
	}

	// If none specified, detect all types
	if len(types) == 0 {
		types = []driftengine.DriftType{
			driftengine.DriftTypeUnmanaged,
			driftengine.DriftTypeOrphaned,
			driftengine.DriftTypeConfig,
		}
	}

	return types
}

// fetchAllStateFiles lists and fetches all state files from the backend
func (a *Agent) fetchAllStateFiles(ctx context.Context, backend backend.StateBackend, emitter protocol.Emitter) (*stateparser.StateFile, error) {
	// List all state files
	stateKeys, err := backend.ListStateFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list state files: %w", err)
	}

	if len(stateKeys) == 0 {
		return nil, fmt.Errorf("no .tfstate files found in container")
	}

	emitter.SendMessage(fmt.Sprintf("  Found %d state file(s) to scan...\n", len(stateKeys)))

	// Fetch and merge all state files
	mergedState := &stateparser.StateFile{
		Version:          4,
		TerraformVersion: "",
		Resources:        []stateparser.StateResource{},
	}

	successCount := 0
	for _, key := range stateKeys {
		// Create a temporary backend config for this specific key
		tempBackend, err := a.createBackendForKey(key)
		if err != nil {
			emitter.SendMessage(fmt.Sprintf("  ⚠ Warning: Failed to create backend for %s: %v\n", key, err))
			continue
		}

		// Fetch this state file
		stateFile, err := tempBackend.FetchState(ctx)
		if err != nil {
			emitter.SendMessage(fmt.Sprintf("  ⚠ Warning: Failed to fetch %s: %v\n", key, err))
			continue
		}

		// Merge resources
		mergedState.Resources = append(mergedState.Resources, stateFile.Resources...)
		successCount++
		emitter.SendMessage(fmt.Sprintf("  ✓ Loaded %s (%d resources)\n", key, len(stateFile.Resources)))
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to fetch any state files")
	}

	emitter.SendMessage(fmt.Sprintf("  Total: %d/%d state files loaded successfully\n", successCount, len(stateKeys)))

	return mergedState, nil
}

// createBackendForKey creates a backend instance for a specific state file key
func (a *Agent) createBackendForKey(key string) (backend.StateBackend, error) {
	config := backend.BackendConfig{
		Type:       a.config.TFStateBackendType,
		Connection: a.config.TFStateBackendConnection,
		Credential: a.config.TFStateBackendCredential,
		Container:  a.config.TFStateContainer,
		Key:        key,
	}
	return backend.NewBackend(config)
}

// emitResults formats and emits the drift detection results
func (a *Agent) emitResults(
	emitter protocol.Emitter,
	findings []driftengine.DriftFinding,
	summary *driftengine.DriftSummary,
	scope cloudquery.ScopeConfig,
) {
	// Emit scope information
	emitter.SendMessage("---\n\n")
	emitter.SendMessage("### Scope\n\n")
	emitter.SendMessage(fmt.Sprintf("- **Subscription**: `%s`\n", scope.SubscriptionID))
	
	if len(scope.ResourceGroups) > 0 {
		emitter.SendMessage(fmt.Sprintf("- **Resource Groups**: %s\n", strings.Join(scope.ResourceGroups, ", ")))
	}
	
	if len(scope.ResourceTypes) > 0 {
		emitter.SendMessage(fmt.Sprintf("- **Resource Types**: %s\n", strings.Join(scope.ResourceTypes, ", ")))
	}
	
	if len(scope.Tags) > 0 {
		var tagStrs []string
		for k, v := range scope.Tags {
			tagStrs = append(tagStrs, fmt.Sprintf("%s=%s", k, v))
		}
		emitter.SendMessage(fmt.Sprintf("- **Tags**: %s\n", strings.Join(tagStrs, ", ")))
	}

	emitter.SendMessage("\n")

	// Emit summary
	emitter.SendMessage("### Summary\n\n")
	emitter.SendMessage(fmt.Sprintf("- **State file resources**: %d\n", summary.StateResources))
	emitter.SendMessage(fmt.Sprintf("- **Azure resources found**: %d\n", summary.AzureResources))
	emitter.SendMessage(fmt.Sprintf("- **Total drift findings**: %d\n", summary.TotalFindings))
	
	if summary.UnmanagedCount > 0 {
		emitter.SendMessage(fmt.Sprintf("  - **Unmanaged resources**: %d (%s)\n", summary.UnmanagedCount, driftengine.SeverityHigh))
	}
	if summary.OrphanedCount > 0 {
		emitter.SendMessage(fmt.Sprintf("  - **Orphaned state entries**: %d (%s)\n", summary.OrphanedCount, driftengine.SeverityMedium))
	}
	if summary.ConfigDriftCount > 0 {
		emitter.SendMessage(fmt.Sprintf("  - **Configuration drift**: %d\n", summary.ConfigDriftCount))
	}

	emitter.SendMessage("\n")

	// If no drift found, emit success message and return
	if len(findings) == 0 {
		emitter.SendMessage("✅ **No drift detected!** Your Terraform state and Azure resources are in sync.\n\n")
		return
	}

	// Group findings by type and emit
	a.emitUnmanagedFindings(emitter, findings)
	a.emitOrphanedFindings(emitter, findings)
	a.emitConfigDriftFindings(emitter, findings)

	// Emit recommendations
	emitter.SendMessage("\n### Recommendations\n\n")
	emitter.SendMessage("1. **Unmanaged resources**: Review and either import with `terraform import` or remove if unnecessary\n")
	emitter.SendMessage("2. **Orphaned state**: Remove from state with `terraform state rm` or recreate if deleted accidentally\n")
	emitter.SendMessage("3. **Configuration drift**: Run `terraform plan` to review and `terraform apply` to reconcile\n")
	emitter.SendMessage("\n")
}

// emitUnmanagedFindings emits unmanaged resource findings
func (a *Agent) emitUnmanagedFindings(emitter protocol.Emitter, findings []driftengine.DriftFinding) {
	unmanaged := filterByType(findings, driftengine.DriftTypeUnmanaged)
	if len(unmanaged) == 0 {
		return
	}

	emitter.SendMessage(fmt.Sprintf("### 🔴 Unmanaged Resources (%d)\n\n", len(unmanaged)))
	emitter.SendMessage("Resources exist in Azure but are NOT managed by Terraform.\n\n")
	emitter.SendMessage("| Resource ID | Type | Resource Group | Location |\n")
	emitter.SendMessage("|-------------|------|----------------|----------|\n")

	for _, finding := range unmanaged {
		emitter.SendMessage(fmt.Sprintf("| `%s` | %s | %s | %s |\n",
			truncate(finding.ResourceID, 60),
			finding.ResourceType,
			finding.ResourceGroup,
			finding.Location,
		))
	}

	emitter.SendMessage("\n")
}

// emitOrphanedFindings emits orphaned state findings
func (a *Agent) emitOrphanedFindings(emitter protocol.Emitter, findings []driftengine.DriftFinding) {
	orphaned := filterByType(findings, driftengine.DriftTypeOrphaned)
	if len(orphaned) == 0 {
		return
	}

	emitter.SendMessage(fmt.Sprintf("### 🟡 Orphaned State Entries (%d)\n\n", len(orphaned)))
	emitter.SendMessage("Resources in Terraform state but NOT found in Azure.\n\n")
	emitter.SendMessage("| State Name | Type | Azure Resource ID |\n")
	emitter.SendMessage("|------------|------|-------------------|\n")

	for _, finding := range orphaned {
		emitter.SendMessage(fmt.Sprintf("| `%s` | %s | `%s` |\n",
			finding.StateName,
			finding.ResourceType,
			truncate(finding.ResourceID, 60),
		))
	}

	emitter.SendMessage("\n")
}

// emitConfigDriftFindings emits configuration drift findings
func (a *Agent) emitConfigDriftFindings(emitter protocol.Emitter, findings []driftengine.DriftFinding) {
	configDrift := filterByType(findings, driftengine.DriftTypeConfig)
	if len(configDrift) == 0 {
		return
	}

	emitter.SendMessage(fmt.Sprintf("### 🔵 Configuration Drift (%d)\n\n", len(configDrift)))
	emitter.SendMessage("Property differences between Terraform state and Azure.\n\n")

	for _, finding := range configDrift {
		emitter.SendMessage(fmt.Sprintf("#### %s (`%s.%s`)\n\n",
			finding.ResourceName,
			finding.ResourceType,
			finding.ResourceName,
		))

		if len(finding.PropertyDiffs) > 0 {
			emitter.SendMessage("| Property | State Value | Azure Value | Severity | Category |\n")
			emitter.SendMessage("|----------|-------------|-------------|----------|----------|\n")

			for _, diff := range finding.PropertyDiffs {
				emitter.SendMessage(fmt.Sprintf("| `%s` | `%v` | `%v` | %s | %s |\n",
					diff.Path,
					formatValue(diff.StateValue),
					formatValue(diff.AzureValue),
					diff.Severity,
					diff.Category,
				))
			}

			emitter.SendMessage("\n")
		}
	}
}

// emitError emits an error message and returns the error
func (a *Agent) emitError(emitter protocol.Emitter, message string, err error) error {
	fullMessage := fmt.Sprintf("❌ **Error**: %s\n\n```\n%v\n```\n\n", message, err)
	emitter.SendMessage(fullMessage)
	
	return fmt.Errorf("%s: %w", message, err)
}

// Helper functions

func filterByType(findings []driftengine.DriftFinding, driftType driftengine.DriftType) []driftengine.DriftFinding {
	var filtered []driftengine.DriftFinding
	for _, finding := range findings {
		if finding.Type == driftType {
			filtered = append(filtered, finding)
		}
	}
	return filtered
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatValue(v interface{}) string {
	if v == nil {
		return "(empty)"
	}
	str := fmt.Sprintf("%v", v)
	if len(str) > 40 {
		return str[:37] + "..."
	}
	return str
}
