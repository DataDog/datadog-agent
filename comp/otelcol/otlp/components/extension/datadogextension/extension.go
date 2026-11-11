// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"context"
	"errors"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/agentcomponents"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
)

type datadogExtension struct {
	metadata        extension.Extension
	rc              extension.Extension
	metadataStarted bool
	rcStarted       bool
	di              *diSender
	startDI         func(context.Context, component.Host) error
}

var (
	_ extensioncapabilities.ConfigSnapshotWatcher = (*datadogExtension)(nil)
	_ extensioncapabilities.PipelineWatcher       = (*datadogExtension)(nil)
	_ componentstatus.Watcher                     = (*datadogExtension)(nil)
)

func (e *datadogExtension) Start(ctx context.Context, host component.Host) error {
	if err := e.metadata.Start(ctx, host); err != nil {
		return err
	}
	e.metadataStarted = true
	if e.startDI != nil {
		if err := e.startDI(ctx, host); err != nil {
			e.metadataStarted = false
			return errors.Join(err, e.metadata.Shutdown(ctx))
		}
	}
	if e.rc == nil {
		return nil
	}
	if err := e.rc.Start(ctx, host); err != nil {
		e.metadataStarted = false
		return errors.Join(err, e.metadata.Shutdown(ctx))
	}
	e.rcStarted = true
	return nil
}

func (e *datadogExtension) Shutdown(ctx context.Context) error {
	var rcErr, metadataErr error
	if e.rcStarted {
		rcErr = e.rc.Shutdown(ctx)
		e.rcStarted = false
	}
	if e.metadataStarted {
		metadataErr = e.metadata.Shutdown(ctx)
		e.metadataStarted = false
	}
	return errors.Join(rcErr, metadataErr)
}

func (e *datadogExtension) NotifyConfigSnapshot(ctx context.Context, snapshot extensioncapabilities.ConfigSnapshot) error {
	if watcher, ok := e.metadata.(extensioncapabilities.ConfigSnapshotWatcher); ok {
		return watcher.NotifyConfigSnapshot(ctx, snapshot)
	}
	return nil
}
func (e *datadogExtension) Ready() error {
	if watcher, ok := e.metadata.(extensioncapabilities.PipelineWatcher); ok {
		return watcher.Ready()
	}
	return nil
}
func (e *datadogExtension) NotReady() error {
	if watcher, ok := e.metadata.(extensioncapabilities.PipelineWatcher); ok {
		return watcher.NotReady()
	}
	return nil
}
func (e *datadogExtension) ComponentStatusChanged(id *componentstatus.InstanceID, event *componentstatus.Event) {
	if watcher, ok := e.metadata.(componentstatus.Watcher); ok {
		watcher.ComponentStatusChanged(id, event)
	}
}
func (e *datadogExtension) GetSerializer() agentcomponents.SerializerWithForwarder {
	if provider, ok := e.metadata.(interface {
		GetSerializer() agentcomponents.SerializerWithForwarder
	}); ok {
		return provider.GetSerializer()
	}
	return nil
}
