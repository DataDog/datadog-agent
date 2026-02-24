// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package tailerfactory implements the logic required to determine which kind
// of tailer to use for a container-related LogSource, and to create that tailer.
package tailerfactory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	compConfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container/tailerfactory/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type kubeUtilMock struct {
	kubelet.KubeUtilInterface
	mock.Mock
}

func TestMakeAPITailer_get_kubeutil_fails(t *testing.T) {
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	pod, container := makeTestPod()
	store.Set(pod)
	store.Set(container)

	tf := &factory{
		workloadmetaStore: option.New[workloadmeta.Component](store),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Identifier: "abc",
	})
	_, err := tf.makeAPITailer(source)
	require.ErrorContains(t, err, "Could not use kubelet client to collect logs for container")
}

func TestMakeAPITailer_success(t *testing.T) {
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	pod, container := makeTestPod()
	store.Set(pod)
	store.Set(container)

	ku := &kubeUtilMock{}
	kubeUtilGet = func() (kubelet.KubeUtilInterface, error) {
		return ku, nil
	}

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		workloadmetaStore: option.New[workloadmeta.Component](store),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Identifier: "abc",
		Service:    "svc",
		Source:     "src",
	})
	at, err := tf.makeAPITailer(source)
	require.NoError(t, err)
	require.Equal(t, source.Config.Identifier, at.(*tailers.APITailer).ContainerID)
}
