// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestSearchTargetPerName(t *testing.T) {
	pod1 := newFakePod(
		"foo",
		"pod1_name",
		"1111",
		"1.1.1.1",
	)

	pod2 := newFakePod(
		"foo",
		"pod2_name",
		"2222",
		"2.2.2.2",
	)

	for nb, tc := range []struct {
		targetName  string
		endpoints   v1.Endpoints
		expectedIP  string
		expectedErr error
	}{
		{
			"pod2_name",
			v1.Endpoints{
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{}, // Empty addr with a nil targetRef
						},
					},
					{
						Addresses: []v1.EndpointAddress{
							newFakeEndpointAddress("myNode", pod1),
							newFakeEndpointAddress("myNode", pod2),
						},
					},
				},
			},
			"2.2.2.2",
			nil,
		},
		{
			"pod_not_found",
			v1.Endpoints{
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							newFakeEndpointAddress("myNode", pod1),
						},
					},
					{
						Addresses: []v1.EndpointAddress{
							newFakeEndpointAddress("myNode", pod2),
						},
					},
				},
			},
			"",
			errors.New("\"target named pod_not_found\" not found"),
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.targetName), func(t *testing.T) {
			target, err := SearchTargetPerName(&tc.endpoints, tc.targetName)
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedIP, target.IP)
			}
		})
	}
}
