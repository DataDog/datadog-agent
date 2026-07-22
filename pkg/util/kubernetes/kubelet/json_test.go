// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestNewPodUnmarshallerIsolation guards against a regression where building
// a PodUnmarshaller would silently clobber an earlier one's decoder.
func TestNewPodUnmarshallerIsolation(t *testing.T) {
	mockConfig := configmock.New(t)

	mockConfig.SetInTest("kubernetes_pod_expiration_duration", 15*60)
	pu1 := NewPodUnmarshaller()

	mockConfig.SetInTest("kubernetes_pod_expiration_duration", 0)
	NewPodUnmarshaller()

	data := []byte(`{"items":[{"status":{"phase":"Succeeded","containerStatuses":[{"state":{"terminated":{"finishedAt":"2018-02-14T14:57:17Z"}}}]}}]}`)
	var podList PodList
	require.NoError(t, pu1.Unmarshal(data, &podList))

	assert.Equal(t, 1, podList.ExpiredCount)
}
