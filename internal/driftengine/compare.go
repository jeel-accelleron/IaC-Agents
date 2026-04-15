package driftengine

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/cloudquery"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

// PropertyComparator handles deep comparison of resource properties
type PropertyComparator struct {
	ignoredProperties map[string]bool
	severityRules     map[string]Severity
	categoryRules     map[string]string
}

// NewPropertyComparator creates a new property comparator with default rules
func NewPropertyComparator() *PropertyComparator {
	return &PropertyComparator{
		ignoredProperties: getDefaultIgnoredProperties(),
		severityRules:     getDefaultSeverityRules(),
		categoryRules:     getDefaultCategoryRules(),
	}
}

// getDefaultIgnoredProperties returns properties that should not be compared
func getDefaultIgnoredProperties() map[string]bool {
	return map[string]bool{
		// System state properties
		"provisioningstate": true,
		"id":                true,
		"etag":              true,
		"systemdata":        true,

		// Timestamps
		"createdtime":      true,
		"lastmodifiedtime": true,
		"changedtime":      true,
		"timestamp":        true,

		// Auto-generated identity
		"principalid": true,
		"tenantid":    true,

		// Computed values
		"fqdn":             true,
		"ipaddress":        true,
		"privateipaddress": true,
		"publicipaddress":  true,
	}
}

// getDefaultSeverityRules returns severity rules for property categories
func getDefaultSeverityRules() map[string]Severity {
	return map[string]Severity{
		"security":   SeverityHigh,
		"networking": SeverityHigh,
		"config":     SeverityMedium,
		"tags":       SeverityLow,
		"metadata":   SeverityLow,
	}
}

// getDefaultCategoryRules returns category classifications for property patterns
func getDefaultCategoryRules() map[string]string {
	return map[string]string{
		// Security-related
		"tls":                    "security",
		"ssl":                    "security",
		"encryption":             "security",
		"https":                  "security",
		"certificate":            "security",
		"key_vault":              "security",
		"access":                 "security",
		"authentication":         "security",
		"rbac":                   "security",
		"public_network_access":  "security",
		"firewall":               "security",

		// Networking-related
		"network":        "networking",
		"subnet":         "networking",
		"vnet":           "networking",
		"nsg":            "networking",
		"endpoint":       "networking",
		"private_link":   "networking",
		"route":          "networking",

		// Tags/Metadata
		"tags":        "tags",
		"label":       "metadata",
		"description": "metadata",
		"display":     "metadata",

		// Everything else is config
	}
}

// CompareProperties compares properties between state and Azure resources
func (pc *PropertyComparator) CompareProperties(
	stateResource stateparser.StateResource,
	cloudResource cloudquery.CloudResource,
) []PropertyDiff {
	var diffs []PropertyDiff

	// Compare basic properties
	diffs = append(diffs, pc.compareLocation(stateResource, cloudResource)...)
	diffs = append(diffs, pc.compareTags(stateResource, cloudResource)...)

	// Compare resource-specific attributes
	diffs = append(diffs, pc.compareAttributes(stateResource, cloudResource)...)

	return diffs
}

// compareLocation compares the location/region
func (pc *PropertyComparator) compareLocation(
	stateResource stateparser.StateResource,
	cloudResource cloudquery.CloudResource,
) []PropertyDiff {
	var diffs []PropertyDiff

	stateLocation := normalizeLocation(stateResource.Location)
	cloudLocation := normalizeLocation(cloudResource.Location)

	if stateLocation != "" && cloudLocation != "" && stateLocation != cloudLocation {
		diffs = append(diffs, PropertyDiff{
			Path:       "location",
			StateValue: stateResource.Location,
			AzureValue: cloudResource.Location,
			Severity:   SeverityMedium,
			Category:   "config",
		})
	}

	return diffs
}

// compareTags compares resource tags
func (pc *PropertyComparator) compareTags(
	stateResource stateparser.StateResource,
	cloudResource cloudquery.CloudResource,
) []PropertyDiff {
	var diffs []PropertyDiff

	// Extract tags from state attributes
	stateTags := make(map[string]string)
	if tagsAttr, ok := stateResource.Attributes["tags"]; ok {
		if tagsMap, ok := tagsAttr.(map[string]interface{}); ok {
			for k, v := range tagsMap {
				if strVal, ok := v.(string); ok {
					stateTags[k] = strVal
				}
			}
		}
	}

	// Compare tags
	allKeys := make(map[string]bool)
	for k := range stateTags {
		allKeys[k] = true
	}
	for k := range cloudResource.Tags {
		allKeys[k] = true
	}

	for key := range allKeys {
		stateVal := stateTags[key]
		cloudVal := cloudResource.Tags[key]

		if stateVal != cloudVal {
			diffs = append(diffs, PropertyDiff{
				Path:       fmt.Sprintf("tags.%s", key),
				StateValue: stateVal,
				AzureValue: cloudVal,
				Severity:   SeverityLow,
				Category:   "tags",
			})
		}
	}

	return diffs
}

// compareAttributes compares resource-specific attributes
func (pc *PropertyComparator) compareAttributes(
	stateResource stateparser.StateResource,
	cloudResource cloudquery.CloudResource,
) []PropertyDiff {
	var diffs []PropertyDiff

	// Normalize and compare each attribute
	for key, stateValue := range stateResource.Attributes {
		normalizedKey := normalizePropertyName(key)

		// Skip ignored properties
		if pc.ignoredProperties[normalizedKey] {
			continue
		}

		// Skip tags (handled separately)
		if normalizedKey == "tags" {
			continue
		}

		// Try to find corresponding Azure property
		azureValue := findAzureProperty(cloudResource.Properties, key)

		// Compare values
		if !areValuesEqual(stateValue, azureValue) {
			diff := PropertyDiff{
				Path:       key,
				StateValue: stateValue,
				AzureValue: azureValue,
			}

			// Determine category and severity
			diff.Category = pc.categorizeProperty(key)
			diff.Severity = pc.severityRules[diff.Category]
			if diff.Severity == "" {
				diff.Severity = SeverityMedium // Default
			}

			diffs = append(diffs, diff)
		}
	}

	return diffs
}

// normalizePropertyName converts property names to a common format for comparison
func normalizePropertyName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)
	
	// Remove underscores and convert from snake_case
	name = strings.ReplaceAll(name, "_", "")
	
	return name
}

// normalizeLocation normalizes Azure location strings
func normalizeLocation(location string) string {
	// Remove spaces and convert to lowercase
	location = strings.ToLower(strings.ReplaceAll(location, " ", ""))
	return location
}

// findAzureProperty finds a property in Azure properties map
// Tries both exact match and normalized camelCase match
func findAzureProperty(azureProps map[string]interface{}, statePropName string) interface{} {
	// Try exact match first
	if val, ok := azureProps[statePropName]; ok {
		return val
	}

	// Try camelCase conversion (e.g., min_tls_version -> minTlsVersion)
	camelCase := snakeToCamelCase(statePropName)
	if val, ok := azureProps[camelCase]; ok {
		return val
	}

	// Try normalized match
	normalizedState := normalizePropertyName(statePropName)
	for azureKey, azureVal := range azureProps {
		if normalizePropertyName(azureKey) == normalizedState {
			return azureVal
		}
	}

	return nil
}

// snakeToCamelCase converts snake_case to camelCase
func snakeToCamelCase(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return s
	}

	result := parts[0]
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			result += strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}

	return result
}

// areValuesEqual compares two values for equality
func areValuesEqual(v1, v2 interface{}) bool {
	if v1 == nil && v2 == nil {
		return true
	}
	if v1 == nil || v2 == nil {
		return false
	}

	// Normalize string values
	if s1, ok1 := v1.(string); ok1 {
		if s2, ok2 := v2.(string); ok2 {
			// Trim whitespace and compare case-insensitively for enums
			return strings.TrimSpace(strings.ToLower(s1)) == strings.TrimSpace(strings.ToLower(s2))
		}
	}

	// Use reflect.DeepEqual for complex types
	return reflect.DeepEqual(v1, v2)
}

// categorizeProperty determines the category of a property
func (pc *PropertyComparator) categorizeProperty(propName string) string {
	normalized := strings.ToLower(propName)

	for pattern, category := range pc.categoryRules {
		if strings.Contains(normalized, pattern) {
			return category
		}
	}

	// Default category
	return "config"
}
