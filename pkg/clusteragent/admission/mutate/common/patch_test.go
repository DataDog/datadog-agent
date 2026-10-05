// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"

	admissioncommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
)

func TestPatchWrapperOutcomes(t *testing.T) {
	for _, mode := range []string{"noop", "annotation", "normalization", "feature error", "ignored patch error"} {
		t.Run(mode, func(t *testing.T) {
			raw := []byte(`{"spec":{"containers":[{"name":"app"}]},"custom":18446744073709551617}`)
			if mode == "normalization" || mode == "feature error" {
				raw = []byte(`{"spec":{"volumes":[{"name":"a","emptyDir":{}},{"name":"a","emptyDir":{}}]}}`)
			}
			calls := 0
			wire, err := MutateWithPatch(raw, "ns", "test", func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
				calls++
				switch mode {
				case "annotation":
					return false, s.SetAnnotations(map[string]string{"blocked": "true"}, false)
				case "feature error":
					if err := s.SetAnnotations(map[string]string{"partial": "true"}, false); err != nil {
						return false, err
					}
					return false, errors.New("feature failed")
				case "ignored patch error":
					_, _ = s.EnsureEnvs([]patch.EnvInjection{{Container: patch.ContainerID{Kind: patch.RegularContainers, Name: "missing"}}})
				}
				return false, nil
			}, nil)
			require.Equal(t, 1, calls, "local validation must not rerun decisions or side effects")
			if mode == "feature error" || mode == "ignored patch error" {
				require.Error(t, err)
				require.Nil(t, wire)
				response := admissioncommon.MutationResponse(wire, err)
				require.True(t, response.Allowed)
				require.Nil(t, response.Patch)
				require.Nil(t, response.PatchType)
			} else {
				require.NoError(t, err)
				if mode == "noop" {
					require.Equal(t, "[]", string(wire))
				} else {
					require.NotEqual(t, "[]", string(wire))
				}
			}
		})
	}
}
