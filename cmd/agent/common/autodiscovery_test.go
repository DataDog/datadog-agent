package common

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)


// Test generated using Keploy
func TestFilterInstances_EmptyInstances(t *testing.T) {
    instances := []integration.Data{}

    validFilter := "name == 'validInstance'"

    filteredInstances, errors := filterInstances(instances, validFilter)
    assert.Empty(t, filteredInstances, "Expected no instances")
    assert.Empty(t, errors, "Expected no errors")
}

// Test generated using Keploy
func TestFilterInstances_InvalidYAMLInstance(t *testing.T) {
    instances := []integration.Data{
        []byte(`invalid yaml`),
    }

    validFilter := "name == 'validInstance'"

    filteredInstances, errors := filterInstances(instances, validFilter)
    assert.NotEmpty(t, errors, "Expected errors due to invalid YAML")
    assert.Empty(t, filteredInstances, "Expected no instances due to invalid YAML")
}
