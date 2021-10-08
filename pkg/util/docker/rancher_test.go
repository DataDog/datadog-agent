// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindRancherIPInLabels(t *testing.T) {
	testCases := []struct {
		labels        map[string]string
		expectedIP    string
		expectedFound bool
	}{
		{
			labels:        map[string]string{},
			expectedIP:    "",
			expectedFound: false,
		},
		{
			labels: map[string]string{
				"io.rancher.container.ip": "10.42.90.224/16",
			},
			expectedIP:    "10.42.90.224",
			expectedFound: true,
		},
		{
			labels: map[string]string{
				"io.rancher.container.ip": "42.90.224/16",
			},
			expectedIP:    "",
			expectedFound: false,
		},
		{
			labels: map[string]string{
				"io.rancher.container.ip": "10.42.90.224",
			},
			expectedIP:    "",
			expectedFound: false,
		},
	}
	for i, test := range testCases {
		t.Run(fmt.Sprintf("case %d: %v", i, test.labels), func(t *testing.T) {
			ip, found := FindRancherIPInLabels(test.labels)
			assert.Equal(t, test.expectedIP, ip)
			assert.Equal(t, test.expectedFound, found)
		})
	}
}
