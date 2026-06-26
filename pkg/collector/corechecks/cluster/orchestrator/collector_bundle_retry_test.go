// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
)

func TestSkipCollectorsForInformerSkipsAllCollectorsSharingInformer(t *testing.T) {
	informer := &fakeSharedInformer{}
	otherInformer := &fakeSharedInformer{}

	clusterCollector := newFakeInformerCollector("cluster", informer)
	nodeCollector := newFakeInformerCollector("nodes", informer)
	namespaceCollector := newFakeInformerCollector("namespaces", otherInformer)

	cb := &CollectorBundle{
		collectors: []collectors.K8sCollector{
			clusterCollector,
			nodeCollector,
			namespaceCollector,
		},
	}

	err := errors.New("cache sync timed out")
	cb.skipCollectorsForInformer(informer, err)

	require.True(t, clusterCollector.Metadata().IsSkipped)
	require.Equal(t, err.Error(), clusterCollector.Metadata().SkippedReason)
	require.True(t, nodeCollector.Metadata().IsSkipped)
	require.Equal(t, err.Error(), nodeCollector.Metadata().SkippedReason)
	require.False(t, namespaceCollector.Metadata().IsSkipped)
}

func TestRecoverSkippedCollectorWaitsForInformerSync(t *testing.T) {
	informer := &fakeSharedInformer{}
	collector := newFakeInformerCollector("namespaces", informer)
	collector.Metadata().IsSkipped = true
	collector.Metadata().SkippedReason = "cache sync timed out"

	cb := &CollectorBundle{}

	require.False(t, cb.recoverSkippedCollector(collector))
	require.True(t, collector.Metadata().IsSkipped)
	require.Equal(t, "cache sync timed out", collector.Metadata().SkippedReason)

	informer.synced = true

	require.True(t, cb.recoverSkippedCollector(collector))
	require.False(t, collector.Metadata().IsSkipped)
	require.Empty(t, collector.Metadata().SkippedReason)
}

type fakeInformerCollector struct {
	metadata *collectors.CollectorMetadata
	informer cache.SharedInformer
}

func newFakeInformerCollector(name string, informer cache.SharedInformer) *fakeInformerCollector {
	return &fakeInformerCollector{
		metadata: &collectors.CollectorMetadata{Name: name},
		informer: informer,
	}
}

func (c *fakeInformerCollector) Init(*collectors.CollectorRunConfig) {}

func (c *fakeInformerCollector) Metadata() *collectors.CollectorMetadata {
	return c.metadata
}

func (c *fakeInformerCollector) Run(*collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	return &collectors.CollectorRunResult{
		Result: processors.ProcessResult{
			MetadataMessages: []model.MessageBody{},
			ManifestMessages: []model.MessageBody{},
		},
	}, nil
}

func (c *fakeInformerCollector) Process(*collectors.CollectorRunConfig, interface{}) (*collectors.CollectorRunResult, error) {
	return c.Run(nil)
}

func (c *fakeInformerCollector) Informer() cache.SharedInformer {
	return c.informer
}

type fakeSharedInformer struct {
	synced bool
}

func (i *fakeSharedInformer) AddEventHandler(cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (i *fakeSharedInformer) AddEventHandlerWithResyncPeriod(cache.ResourceEventHandler, time.Duration) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (i *fakeSharedInformer) AddEventHandlerWithOptions(cache.ResourceEventHandler, cache.HandlerOptions) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func (i *fakeSharedInformer) RemoveEventHandler(cache.ResourceEventHandlerRegistration) error {
	return nil
}

func (i *fakeSharedInformer) GetStore() cache.Store {
	return nil
}

func (i *fakeSharedInformer) GetController() cache.Controller {
	return nil
}

func (i *fakeSharedInformer) Run(<-chan struct{}) {}

func (i *fakeSharedInformer) RunWithContext(context.Context) {}

func (i *fakeSharedInformer) HasSynced() bool {
	return i.synced
}

func (i *fakeSharedInformer) LastSyncResourceVersion() string {
	return ""
}

func (i *fakeSharedInformer) SetWatchErrorHandler(cache.WatchErrorHandler) error {
	return nil
}

func (i *fakeSharedInformer) SetWatchErrorHandlerWithContext(cache.WatchErrorHandlerWithContext) error {
	return nil
}

func (i *fakeSharedInformer) SetTransform(cache.TransformFunc) error {
	return nil
}

func (i *fakeSharedInformer) IsStopped() bool {
	return false
}
