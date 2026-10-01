// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package taggerimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	clusteridresolvermock "github.com/DataDog/datadog-agent/comp/core/clusteridresolver/mock"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newClusterIDTagger(t *testing.T) (*localTagger, *clusteridresolvermock.Mock, *compdef.TestLifecycle) {
	t.Helper()
	flavor.SetTestFlavor(t, flavor.ClusterAgent)
	cfg := config.NewMock(t)
	logger := logmock.New(t)
	store := fxutil.Test[workloadmeta.Component](t,
		fx.Provide(func() config.Component { return cfg }),
		fx.Provide(func() log.Component { return logger }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	lc := compdef.NewTestLifecycle(t)
	resolver := clusteridresolvermock.New()
	provided, err := NewComponent(Requires{Lc: lc, Config: cfg, Log: logger, WorkloadMeta: store,
		Telemetry: noopTelemetry.GetCompatComponent(), ClusterIDResolver: resolver})
	require.NoError(t, err)
	require.NoError(t, lc.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, lc.Stop(context.Background())) })
	return provided.Comp.(*localTagger), resolver, lc
}

func TestClusterIDPublicationGatesHealth(t *testing.T) {
	tagger, resolver, _ := newClusterIDTagger(t)
	for _, status := range []health.Status{health.GetStartup(), health.GetLive(), health.GetReady()} {
		assert.Contains(t, status.Unhealthy, "cluster-id-global-tags")
	}
	other := health.RegisterReadiness("other-component")
	t.Cleanup(func() { require.NoError(t, other.Deregister()) })
	const id = "226430c6-5e57-11ea-91d5-42010a8400c6"
	resolver.SetID(id)
	select {
	case <-tagger.clusterIDPublicationDone:
	case <-time.After(5 * time.Second):
		t.Fatal("cluster ID publication did not finish")
	}
	tags, err := tagger.GlobalTags(types.LowCardinality)
	require.NoError(t, err)
	assert.Contains(t, tags, "orch_cluster_id:"+id)
	assert.Contains(t, health.GetReady().Unhealthy, "other-component")
	for _, status := range []health.Status{health.GetStartup(), health.GetLive(), health.GetReady()} {
		assert.NotContains(t, status.Unhealthy, "cluster-id-global-tags")
	}
}

func TestClusterIDPublicationStopsWhileUnresolved(t *testing.T) {
	tagger, _, lc := newClusterIDTagger(t)
	assert.Contains(t, health.GetReady().Unhealthy, "cluster-id-global-tags")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, lc.Stop(ctx))
	select {
	case <-tagger.clusterIDPublicationDone:
	default:
		t.Fatal("publication worker must finish before shutdown returns")
	}
	for _, status := range []health.Status{health.GetStartup(), health.GetLive(), health.GetReady()} {
		assert.NotContains(t, status.Unhealthy, "cluster-id-global-tags")
	}
}
