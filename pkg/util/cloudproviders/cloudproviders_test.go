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

	aliases := GetHostAliases(context.TODO())
	assert.True(t, detector1Called, "host alias callback for 'detector1' was not called")
	assert.True(t, detector2Called, "host alias callback for 'detector2' was not called")
	assert.True(t, detector3Called, "host alias callback for 'detector3' was not called")

	assert.Equal(t, []string{"alias1", "alias2"}, aliases)

}
