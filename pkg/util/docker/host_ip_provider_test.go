// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProvider(t *testing.T) {

	providersCalled := make(map[int]bool)

	providers := []hostIPProvider{
		{
			provider: func() ([]string, error) {
				providersCalled[0] = true
				return nil, fmt.Errorf("provider 0 error")
			},
		},
		{
			provider: func() ([]string, error) {
				providersCalled[1] = true
				return []string{"10.0.0.1", "10.0.0.2"}, nil
			},
		},
		{
			provider: func() ([]string, error) {
				providersCalled[2] = true
				return nil, fmt.Errorf("provider 2 error")
			},
		},
	}

	ips := tryProviders(providers)

	assert.Len(t, ips, 2)
	assert.ElementsMatch(t, []string{"10.0.0.1", "10.0.0.2"}, ips)
	assert.True(t, providersCalled[0])
	assert.True(t, providersCalled[1])
	assert.False(t, providersCalled[2])
}
