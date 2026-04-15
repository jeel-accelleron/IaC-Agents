# Cloud Drift Detection Workflow

## Overview

The Cloud Drift Agent detects discrepancies between Terraform state files and actual Azure resources. It identifies three types of drift:

1. **Unmanaged Resources** (HIGH) - Resources exist in Azure but are NOT managed by Terraform
2. **Orphaned State Entries** (MEDIUM) - Resources in Terraform state but NOT found in Azure  
3. **Configuration Drift** - Property differences between Terraform state and Azure

---

## Workflow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     Cloud Drift Detection                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Step 1: Fetch Terraform State File(s)                           │
├─────────────────────────────────────────────────────────────────┤
│ • Connect to backend (Azure Blob / S3 / Local)                  │
│ • Single file mode: Fetch TF_STATE_BACKEND_KEY                  │
│ • Multi-scan mode: List all .tfstate files (TF_STATE_SCAN_ALL)  │
│ • Parse state files (Terraform JSON v4+)                        │
│ • Extract managed resources with Azure IDs                      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Step 2: Query Azure Resources                                   │
├─────────────────────────────────────────────────────────────────┤
│ • Authenticate with Azure (DefaultAzureCredential)              │
│ • Build Resource Graph query (KQL) based on scope:              │
│   - Subscription ID (required)                                  │
│   - Resource Groups (optional filter)                           │
│   - Resource Types (optional filter)                            │
│   - Tags (optional filter)                                      │
│ • Execute query with pagination (1000 resources/page)           │
│ • Extract resource properties and metadata                      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Step 3: Analyze Drift                                           │
├─────────────────────────────────────────────────────────────────┤
│ • Build lookup maps:                                            │
│   - State resources by Azure ID (lowercase normalized)          │
│   - Azure resources by ID                                       │
│                                                                  │
│ • Detect UNMANAGED resources:                                   │
│   - Azure resource ID NOT in state map                          │
│   - Severity: HIGH                                              │
│                                                                  │
│ • Detect ORPHANED state:                                        │
│   - State resource ID NOT in Azure map                          │
│   - Severity: MEDIUM                                            │
│                                                                  │
│ • Detect CONFIG drift:                                          │
│   - Resource exists in both state and Azure                     │
│   - Compare properties with normalization:                      │
│     * Flatten nested objects                                    │
│     * Normalize array order (tags, properties)                  │
│     * Ignore computed/metadata fields                           │
│   - Categorize differences:                                     │
│     * security - Encryption, authentication, access controls    │
│     * networking - IP ranges, DNS, NSG rules, subnets           │
│     * config - General configuration properties                 │
│   - Severity based on category                                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Step 4: Generate Report                                         │
├─────────────────────────────────────────────────────────────────┤
│ • Format markdown report with:                                  │
│   - Scope summary (subscription, filters)                       │
│   - Statistics (total resources, findings)                      │
│   - Unmanaged resources table (ID, type, RG, location)          │
│   - Orphaned state table (state name, type, Azure ID)           │
│   - Config drift details (property-by-property comparison)      │
│   - Recommendations for remediation                             │
│ • Stream report via SSE (Server-Sent Events)                    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Detailed Component Flow

### 1. Backend Connection

**Module**: `internal/backend/`

```go
// Supported backends
type StateBackend interface {
    FetchState(ctx) (*StateFile, error)
    ListStateFiles(ctx) ([]string, error)
}

// Implementations:
- AzureBlobBackend   // Azure Blob Storage (connection string or URL + credential)
- S3Backend          // AWS S3 
- LocalBackend       // Local filesystem
```

**Authentication Methods**:
- **Azure Blob**: 
  - URL + DefaultAzureCredential (Azure CLI, Managed Identity, Service Principal)
  - Connection String (AccountKey)
- **S3**: AWS credentials chain
- **Local**: File path

**Multi-State Scanning** (`TF_STATE_SCAN_ALL=true`):
```
1. ListStateFiles() → Enumerate all .tfstate files in container/bucket
2. For each state file:
   - Create backend with specific key
   - FetchState() → Parse JSON
   - Merge resources into unified map
3. Deduplicate by Azure resource ID
```

---

### 2. State Parsing

**Module**: `internal/stateparser/`

**Process**:
```
1. Validate Terraform format version (≥4)
2. Extract resources from state.values.root_module.resources
3. For each resource:
   - Type: azurerm_* (e.g., azurerm_resource_group)
   - Name: Resource identifier in Terraform
   - Values: Configuration from state
   - Mode: "managed" (ignore data sources)
4. Extract Azure Resource ID from values.id
5. Normalize to lowercase for comparison
```

**Supported Resource Types**:
- All `azurerm_*` providers
- Auto-detects Azure resource IDs in format:  
  `/subscriptions/{sub}/resourceGroups/{rg}/providers/{provider}/{type}/{name}`

---

### 3. Azure Resource Graph Query

**Module**: `internal/cloudquery/`

**Query Construction** (KQL):
```kql
Resources
| where resourceGroup in~ ('rg-prod', 'rg-staging')  // Optional filter
| where type =~ 'microsoft.storage/storageaccounts'   // Optional filter
| where tags['environment'] =~ 'production'           // Optional filter
| project id, type, name, resourceGroup, location, subscriptionId, tags, properties
```

**Filtering Options**:
- `DRIFT_SCOPE_RESOURCE_GROUPS` - Comma-separated RG names
- `DRIFT_SCOPE_RESOURCE_TYPES` - e.g., `Microsoft.Storage/*,Microsoft.Compute/virtualMachines`
- `DRIFT_SCOPE_TAGS` - Key-value pairs (format: `key1=value1,key2=value2`)

**Pagination**:
- 1000 resources per page (Azure limit)
- Automatic continuation with `skipToken`

---

### 4. Drift Detection Engine

**Module**: `internal/driftengine/`

#### Unmanaged Resources Detection
```go
for _, azureResource := range azureResources {
    normalizedID := strings.ToLower(azureResource.ID)
    if _, exists := stateMap[normalizedID]; !exists {
        findings = append(findings, Finding{
            Type:     "unmanaged",
            Severity: "HIGH",
            Resource: azureResource,
        })
    }
}
```

#### Orphaned State Detection
```go
for _, stateResource := range stateResources {
    normalizedID := strings.ToLower(stateResource.ID)
    if _, exists := azureMap[normalizedID]; !exists {
        findings = append(findings, Finding{
            Type:     "orphaned",
            Severity: "MEDIUM",
            StateResource: stateResource,
        })
    }
}
```

#### Configuration Drift Detection
```go
for id, stateResource := range stateMap {
    if azureResource, exists := azureMap[id]; exists {
        diffs := compareProperties(stateResource, azureResource)
        if len(diffs) > 0 {
            findings = append(findings, Finding{
                Type:       "config",
                Resource:   azureResource,
                StateName:  stateResource.Name,
                Diffs:      diffs,
            })
        }
    }
}
```

**Property Comparison**:
- Flatten nested objects: `encryption.enabled` → `encryption_enabled`
- Normalize property names: `resourceGroupName` → `resource_group_name`
- Type conversion: String "true" → Boolean true
- Array order normalization for tags/maps
- Skip computed fields: `id`, `etag`, `timeCreated`, etc.

**Severity Assignment**:
- **HIGH**: Security-related (encryption, authentication, access)
- **MEDIUM**: Networking (IP ranges, DNS, NSG rules), General config
- **LOW**: Metadata, non-critical properties

---

### 5. Report Generation

**Module**: `agents/cloud-drift/`

**Output Format**: Markdown with tables

**Sections**:
1. **Scope** - Subscription, filters applied
2. **Summary** - Total counts by drift type
3. **Unmanaged Resources** - Table with ID, type, RG, location
4. **Orphaned State** - Table with state name, type, Azure ID
5. **Configuration Drift** - Per-resource property comparison tables
6. **Recommendations** - Remediation actions

**Streaming**: Server-Sent Events (SSE) for real-time progress

---

## Configuration

### Environment Variables

```bash
# Azure Authentication
AZURE_SUBSCRIPTION_ID=<subscription-id>
AZURE_CLIENT_ID=<service-principal-client-id>        # Optional
AZURE_CLIENT_SECRET=<service-principal-secret>       # Optional
AZURE_TENANT_ID=<tenant-id>

# Backend Configuration
TF_STATE_BACKEND_TYPE=azureblob|s3|local
TF_STATE_BACKEND_CONNECTION=<url-or-connection-string>
TF_STATE_CONTAINER=<container-or-bucket-name>
TF_STATE_BACKEND_KEY=<state-file-key>                # Single file mode
TF_STATE_SCAN_ALL=true|false                          # Multi-file mode

# Drift Scope (all optional)
DRIFT_SCOPE_RESOURCE_GROUPS=rg-prod,rg-staging
DRIFT_SCOPE_RESOURCE_TYPES=Microsoft.Storage/*,Microsoft.Compute/virtualMachines
DRIFT_SCOPE_TAGS=environment=production,team=platform

# Drift Types to Detect (default: all)
DRIFT_TYPES=unmanaged,orphaned,config
```

### Azure Permissions Required

**Service Principal / User Account needs**:
1. **Storage Blob Data Reader** - On storage account containing state files
2. **Reader** - On subscription/resource groups to query (control plane)
3. **Resource Graph Reader** (built-in role) - Optional, included in Reader

**Grant permissions**:
```bash
# Storage access
az role assignment create \
  --role "Storage Blob Data Reader" \
  --assignee <client-id-or-user-object-id> \
  --scope "/subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Storage/storageAccounts/<account>"

# Subscription-wide query access (included with Reader)
az role assignment create \
  --role "Reader" \
  --assignee <client-id-or-user-object-id> \
  --scope "/subscriptions/<sub-id>"
```

---

## Usage Examples

### Single State File Mode
```bash
TF_STATE_BACKEND_TYPE=azureblob
TF_STATE_BACKEND_CONNECTION=https://myaccount.blob.core.windows.net/
TF_STATE_CONTAINER=tfstate
TF_STATE_BACKEND_KEY=prod/terraform.tfstate
```

### Multi-State Scan Mode
```bash
TF_STATE_BACKEND_TYPE=azureblob
TF_STATE_BACKEND_CONNECTION=https://myaccount.blob.core.windows.net/
TF_STATE_CONTAINER=tfstate
TF_STATE_SCAN_ALL=true  # Scans all .tfstate files in container
```

### Filtered Scope
```bash
# Only scan specific resource groups
DRIFT_SCOPE_RESOURCE_GROUPS=rg-prod,rg-staging

# Only specific resource types
DRIFT_SCOPE_RESOURCE_TYPES=Microsoft.Storage/storageAccounts,Microsoft.Network/*

# Only resources with specific tags
DRIFT_SCOPE_TAGS=environment=production,managed-by=terraform
```

### Detect Specific Drift Types
```bash
# Only unmanaged resources (resources created outside Terraform)
DRIFT_TYPES=unmanaged

# Unmanaged + orphaned only (skip config drift)
DRIFT_TYPES=unmanaged,orphaned
```

---

## API Usage

### HTTP Endpoint
```bash
POST http://localhost:8080/agent/cloud-drift
Content-Type: application/json

{
  "messages": [
    {
      "role": "user",
      "content": "Detect cloud drift"
    }
  ]
}
```

### Response (SSE Stream)
```
event: copilot_message
data: {"choices":[{"delta":{"content":"## Cloud Drift Detection\n\n","role":"assistant"},"index":0}]}

event: copilot_message
data: {"choices":[{"delta":{"content":"**Step 1/4:** Fetching Terraform state file(s)...\n","role":"assistant"},"index":0}]}

event: copilot_message
data: {"choices":[{"delta":{"content":"✓ State file(s) loaded: 19 resources found\n\n","role":"assistant"},"index":0}]}

...

event: copilot_done
data: {}
```

---

## Troubleshooting

### Common Issues

#### 1. Authentication Error: "connection string is either blank or malformed"
**Cause**: Using URL instead of connection string, or vice versa  
**Fix**: Code detects `https://` prefix to use DefaultAzureCredential

#### 2. "403 This request is not authorized"
**Cause**: Missing Storage Blob Data Reader role  
**Fix**: 
```bash
az role assignment create --role "Storage Blob Data Reader" \
  --assignee <user-or-sp-id> \
  --scope "/subscriptions/<sub>/resourceGroups/<rg>/providers/Microsoft.Storage/storageAccounts/<account>"
```

#### 3. "Invalid client secret provided"
**Cause**: Using secret ID instead of secret value  
**Fix**: Copy the **value** (not ID) from Azure Portal when creating client secret

#### 4. "Failed to resolve scalar expression named 'createdTime'"
**Cause**: Azure Resource Graph query includes non-existent fields  
**Fix**: Use only supported fields: `id, type, name, resourceGroup, location, subscriptionId, tags, properties`

#### 5. All resources showing as "orphaned" with 0 Azure resources
**Cause**: `DRIFT_SCOPE_RESOURCE_GROUPS` filter excludes all resources  
**Fix**: Remove or update filter to include actual resource group names

---

## Performance Considerations

### Large State Files (1000+ resources)
- State parsing: ~100ms per file
- Merging: Deduplicated by resource ID
- Memory: ~1MB per 1000 resources

### Azure Resource Graph Limits
- Max 1000 results per page
- Pagination handled automatically
- Query timeout: 90 seconds
- Recommended: Use filters to reduce scope

### Optimization Tips
1. Use `DRIFT_SCOPE_RESOURCE_GROUPS` to limit query scope
2. Use `DRIFT_TYPES=unmanaged` to skip config drift comparison
3. Single file mode is faster than multi-scan for targeted checks
4. Resource Graph queries are faster with specific type filters

---

## Architecture Decisions

### Why Azure Resource Graph instead of Azure REST APIs?
- Single query returns all resources (no need to iterate by type)
- Built-in filtering, pagination, and performance
- Supports complex queries across subscriptions
- Consistent schema for all resource types

### Why multi-state scanning?
- Real-world scenarios: Multiple environments (dev, staging, prod)
- Separate state files per project/team
- Consolidated drift view across all managed infrastructure

### Why normalize resource IDs?
- Azure IDs are case-insensitive
- Terraform may store different casing than Azure returns
- Lowercase normalization ensures reliable matching

### Why three drift types?
- **Unmanaged** = Shadow IT / manual provisioning (highest risk)
- **Orphaned** = State cleanup needed (medium risk)
- **Config** = External modifications (varies by property)

---

## Future Enhancements

- [ ] Support for AWS (CloudFormation/Terraform) drift detection
- [ ] Support for GCP drift detection
- [ ] Remediation automation (auto-import, auto-remove from state)
- [ ] Drift history tracking (detect when drift occurred)
- [ ] Notification integrations (Slack, Teams, email)
- [ ] Scheduled drift scans (cron/timer trigger)
- [ ] Drift policies (allow/deny lists for unmanaged resources)
- [ ] Multi-subscription scanning
- [ ] Multi-tenant support
