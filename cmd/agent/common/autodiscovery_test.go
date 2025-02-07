package common

import (
    "testing"
    "github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)


// Test generated using Keploy
func TestFilterInstances_EmptyInstances(t *testing.T) {
    instances := []integration.Data{}

    validFilter := "name == 'validInstance'"

    filteredInstances, errors := filterInstances(instances, validFilter)
    if len(filteredInstances) != 0 {
        t.Errorf("Expected no instances, got: %v", filteredInstances)
    }
    if len(errors) != 0 {
        t.Errorf("Expected no errors, got: %v", errors)
    }
}

// Test generated using Keploy
func TestFilterInstances_InvalidYAMLInstance(t *testing.T) {
    instances := []integration.Data{
        []byte(`invalid yaml`),
    }

    validFilter := "name == 'validInstance'"

    filteredInstances, errors := filterInstances(instances, validFilter)
    if len(errors) == 0 {
        t.Errorf("Expected errors due to invalid YAML, got none")
    }
    if len(filteredInstances) != 0 {
        t.Errorf("Expected no instances due to invalid YAML, got: %v", filteredInstances)
    }
}

