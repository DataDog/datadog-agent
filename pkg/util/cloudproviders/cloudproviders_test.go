// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudproviders

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloudProviderAliases(t *testing.T) {
	origDetectors := hostAliasesDetectors
	defer func() { hostAliasesDetectors = origDetectors }()

	detector1Called := false
	detector2Called := false
	detector3Called := false

	hostAliasesDetectors = []cloudProviderAliasesDetector{
		{
			name: "detector1",
			callback: func(_ context.Context) ([]string, error) {
				detector1Called = true
				return []string{"alias2"}, nil
			},
		},
		{
			name: "detector2",
			callback: func(_ context.Context) ([]string, error) {
				detector2Called = true
				return nil, fmt.Errorf("error from detector2")
			},
		},
		{
			name: "detector3",
			callback: func(_ context.Context) ([]string, error) {
				detector3Called = true
				return []string{"alias1", "alias2"}, nil
			},
		},
	}

	aliases, cloudprovider := GetHostAliases(context.TODO())
	assert.True(t, detector1Called, "host alias callback for 'detector1' was not called")
	assert.True(t, detector2Called, "host alias callback for 'detector2' was not called")
	assert.True(t, detector3Called, "host alias callback for 'detector3' was not called")

	assert.Equal(t, []string{"alias1", "alias2"}, aliases)
	// Which detector wins depends upon timing, either one is fine
	// In reality we expect only 1 possible cloudprovider to return host aliases
	assert.Contains(t, []string{"detector1", "detector3"}, cloudprovider)
}

func TestCloudProviderHostCCRID(t *testing.T) {
	origDetectors := hostCCRIDDetectors
	defer func() { hostCCRIDDetectors = origDetectors }()

	detector1Called := false
	detector2Called := false

	hostCCRIDDetectors = map[string]cloudProviderCCRIDDetector{
		"detector1": func(_ context.Context) (string, error) {
			detector1Called = true
			return "ccrid1", nil
		},
		"detector2": func(_ context.Context) (string, error) {
			detector2Called = true
			return "ccrid2", nil
		},
	}

	ccrid := GetHostCCRID(context.TODO(), "detector2")
	assert.False(t, detector1Called, "host alias callback for 'detector1' should not be called")
	assert.True(t, detector2Called, "host alias callback for 'detector2' was not called")
	assert.Equal(t, "ccrid2", ccrid)
	detector2Called = false

	ccrid = GetHostCCRID(context.TODO(), "detector1")
	assert.True(t, detector1Called, "host alias callback for 'detector1' was not called")
	assert.False(t, detector2Called, "host alias callback for 'detector2' should not be called")
	assert.Equal(t, "ccrid1", ccrid)
}
