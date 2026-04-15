package cloudquery

import "time"

// CloudResource represents a resource discovered in Azure
type CloudResource struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Name          string                 `json:"name"`
	Location      string                 `json:"location"`
	ResourceGroup string                 `json:"resourceGroup"`
	SubscriptionID string                `json:"subscriptionId"`
	Tags          map[string]string      `json:"tags"`
	Properties    map[string]interface{} `json:"properties"`
	CreatedTime   *time.Time             `json:"createdTime,omitempty"`
	ChangedTime   *time.Time             `json:"changedTime,omitempty"`
}

// ScopeConfig defines the scope for Azure resource queries
type ScopeConfig struct {
	SubscriptionID string
	ResourceGroups []string
	Tags           map[string]string
	ResourceTypes  []string
}

// QueryResult represents the result of an Azure Resource Graph query
type QueryResult struct {
	TotalRecords int64
	Resources    []CloudResource
	SkipToken    string // For pagination
}
