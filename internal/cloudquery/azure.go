package cloudquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

// AzureClient wraps the Azure Resource Graph client
type AzureClient struct {
	client *armresourcegraph.Client
	cred   azcore.TokenCredential
}

// NewAzureClient creates a new Azure Resource Graph client
func NewAzureClient(ctx context.Context) (*AzureClient, error) {
	// Use DefaultAzureCredential which tries multiple authentication methods
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	client, err := armresourcegraph.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Resource Graph client: %w", err)
	}

	return &AzureClient{
		client: client,
		cred:   cred,
	}, nil
}

// NewAzureClientWithCredential creates a client with specific credentials
func NewAzureClientWithCredential(cred azcore.TokenCredential) (*AzureClient, error) {
	client, err := armresourcegraph.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Resource Graph client: %w", err)
	}

	return &AzureClient{
		client: client,
		cred:   cred,
	}, nil
}

// QueryResources queries Azure resources based on the provided scope
func (ac *AzureClient) QueryResources(ctx context.Context, scope ScopeConfig) ([]CloudResource, error) {
	query := buildResourceGraphQuery(scope)
	
	var allResources []CloudResource
	var skipToken *string

	// Handle pagination
	for {
		result, err := ac.executeQuery(ctx, query, scope.SubscriptionID, skipToken)
		if err != nil {
			return nil, err
		}

		resources, err := parseQueryResult(result)
		if err != nil {
			return nil, fmt.Errorf("failed to parse query result: %w", err)
		}

		allResources = append(allResources, resources...)

		// Check for more pages
		if result.SkipToken == nil || *result.SkipToken == "" {
			break
		}
		skipToken = result.SkipToken
	}

	return allResources, nil
}

// executeQuery executes a Resource Graph query
func (ac *AzureClient) executeQuery(
	ctx context.Context,
	query string,
	subscriptionID string,
	skipToken *string,
) (*armresourcegraph.QueryResponse, error) {
	
	request := armresourcegraph.QueryRequest{
		Query: &query,
		Subscriptions: []*string{
			&subscriptionID,
		},
		Options: &armresourcegraph.QueryRequestOptions{
			ResultFormat: to(armresourcegraph.ResultFormatObjectArray),
			SkipToken:    skipToken,
			Top:          to(int32(1000)), // Max results per page
		},
	}

	resp, err := ac.client.Resources(ctx, request, nil)
	if err != nil {
		return nil, fmt.Errorf("Resource Graph query failed: %w", err)
	}

	return &resp.QueryResponse, nil
}

// buildResourceGraphQuery constructs a KQL query based on the scope configuration
func buildResourceGraphQuery(scope ScopeConfig) string {
	var filters []string

	// Base query
	query := "Resources"

	// Filter by resource groups
	if len(scope.ResourceGroups) > 0 {
		rgList := make([]string, len(scope.ResourceGroups))
		for i, rg := range scope.ResourceGroups {
			rgList[i] = fmt.Sprintf("'%s'", strings.ToLower(rg))
		}
		filters = append(filters, fmt.Sprintf("resourceGroup in~ (%s)", strings.Join(rgList, ", ")))
	}

	// Filter by resource types
	if len(scope.ResourceTypes) > 0 {
		typeFilters := make([]string, 0)
		for _, rt := range scope.ResourceTypes {
			if strings.HasSuffix(rt, "/*") {
				// Wildcard match
				prefix := strings.TrimSuffix(rt, "/*")
				typeFilters = append(typeFilters, fmt.Sprintf("type startswith '%s/'", prefix))
			} else {
				// Exact match
				typeFilters = append(typeFilters, fmt.Sprintf("type =~ '%s'", rt))
			}
		}
		if len(typeFilters) > 0 {
			filters = append(filters, fmt.Sprintf("(%s)", strings.Join(typeFilters, " or ")))
		}
	}

	// Filter by tags
	if len(scope.Tags) > 0 {
		for key, value := range scope.Tags {
			if value == "" {
				// Tag key exists
				filters = append(filters, fmt.Sprintf("tags has '%s'", key))
			} else {
				// Tag key and value match
				filters = append(filters, fmt.Sprintf("tags['%s'] =~ '%s'", key, value))
			}
		}
	}

	// Add all filters
	if len(filters) > 0 {
		query += "\n| where " + strings.Join(filters, " and ")
	}

	// Project the fields we need
	query += `
| project id, type, name, resourceGroup, location, subscriptionId, tags, properties`

	return query
}

// parseQueryResult parses the Resource Graph query result into CloudResource objects
func parseQueryResult(result *armresourcegraph.QueryResponse) ([]CloudResource, error) {
	if result.Data == nil {
		return []CloudResource{}, nil
	}

	// Marshal and unmarshal to convert to our type
	dataBytes, err := json.Marshal(result.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result data: %w", err)
	}

	var rawResources []map[string]interface{}
	if err := json.Unmarshal(dataBytes, &rawResources); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result data: %w", err)
	}

	resources := make([]CloudResource, 0, len(rawResources))
	for _, raw := range rawResources {
		resource := CloudResource{
			ID:             getStringField(raw, "id"),
			Type:           getStringField(raw, "type"),
			Name:           getStringField(raw, "name"),
			Location:       getStringField(raw, "location"),
			ResourceGroup:  getStringField(raw, "resourceGroup"),
			SubscriptionID: getStringField(raw, "subscriptionId"),
			Tags:           getTagsField(raw, "tags"),
			Properties:     getMapField(raw, "properties"),
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// Helper functions

func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getTagsField(m map[string]interface{}, key string) map[string]string {
	tags := make(map[string]string)
	if v, ok := m[key]; ok {
		if tagsMap, ok := v.(map[string]interface{}); ok {
			for k, val := range tagsMap {
				if s, ok := val.(string); ok {
					tags[k] = s
				}
			}
		}
	}
	return tags
}

func getMapField(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if mapVal, ok := v.(map[string]interface{}); ok {
			return mapVal
		}
	}
	return make(map[string]interface{})
}

func to[T any](v T) *T {
	return &v
}

// GetResourcesByIDs builds a map of resources by their Azure resource IDs
func GetResourcesByIDs(resources []CloudResource) map[string]CloudResource {
	resourceMap := make(map[string]CloudResource)
	for _, resource := range resources {
		normalizedID := strings.ToLower(resource.ID)
		resourceMap[normalizedID] = resource
	}
	return resourceMap
}
