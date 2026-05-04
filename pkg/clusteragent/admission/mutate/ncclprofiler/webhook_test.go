// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestWebhook_DisabledByDefault(t *testing.T) {
	cfg := configmock.New(t)

	w := NewWebhook(cfg)

	assert.False(t, w.IsEnabled(), "webhook must be disabled when no admission_controller.nccl_profiler.enabled is set")
}

func TestWebhook_EnabledButNoInjectorImage_DisablesItself(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("admission_controller.nccl_profiler.enabled", true)
	// admission_controller.nccl_profiler.injector_image deliberately left empty

	w := NewWebhook(cfg)

	assert.False(t, w.IsEnabled(),
		"webhook must self-disable when enabled=true but injector_image is empty (no broken image refs in pods)")
}

func TestWebhook_EnabledWithInjectorImage(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("admission_controller.nccl_profiler.enabled", true)
	cfg.SetWithoutSource("admission_controller.nccl_profiler.injector_image",
		"376334461865.dkr.ecr.us-east-1.amazonaws.com/nccl-profiler-injector:latest")

	w := NewWebhook(cfg)

	assert.True(t, w.IsEnabled())
	assert.Equal(t, "nccl_profiler", w.Name())
	assert.Equal(t, "/inject-nccl-profiler", w.Endpoint())
}

func TestWebhook_LabelSelectorTargetsOnlyOptedInPods(t *testing.T) {
	cfg := configmock.New(t)
	w := NewWebhook(cfg)

	nsSel, objSel := w.LabelSelectors(false)

	assert.Nil(t, nsSel, "namespace selector should be nil - webhook is opt-in per pod, not per namespace")
	assert.NotNil(t, objSel)
	assert.Equal(t, "true", objSel.MatchLabels[EnabledLabel],
		"object selector must require admission.datadoghq.com/nccl-profiler.enabled=true")
}
