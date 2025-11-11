// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestMutatorCoreNewInjector(t *testing.T) {
	mockConfig := configmock.New(t)
	wmeta := fxutil.Test[workloadmeta.Component](t,
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
	config, err := NewConfig(mockConfig)
	require.NoError(t, err)
	m, err := NewTargetMutator(config, wmeta, imageResolver)
	require.NoError(t, err)
	core := m.core

	// common vars
	startTime := time.Now()
	pod := &corev1.Pod{}

	i := core.newInjector(pod, startTime, libRequirementOptions{})
	require.Equal(t, &injector{
		injectTime: startTime,
		registry:   core.config.containerRegistry,
		image:      core.config.containerRegistry + "/apm-inject:0",
	}, i)

	core.config.Instrumentation.InjectorImageTag = "banana"
	i = core.newInjector(pod, startTime, libRequirementOptions{})
	require.Equal(t, &injector{
		injectTime: startTime,
		registry:   core.config.containerRegistry,
		image:      core.config.containerRegistry + "/apm-inject:banana",
	}, i)
}
