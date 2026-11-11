// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datadogexporter

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/plog"
)

type diEventConsumer interface {
	ConsumeDIEvents(context.Context, plog.Logs) error
}

type diRoutingExporter struct {
	exporter.Logs
	di diEventConsumer
}

func (e *diRoutingExporter) Start(ctx context.Context, host component.Host) error {
	for id, ext := range host.GetExtensions() {
		if id.Type().String() != "datadog" {
			continue
		}
		if di, ok := ext.(diEventConsumer); ok {
			if e.di != nil {
				return errors.New("DI routing requires a single Datadog extension")
			}
			e.di = di
		}
	}
	return e.Logs.Start(ctx, host)
}

func (*diRoutingExporter) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (e *diRoutingExporter) ConsumeLogs(ctx context.Context, input plog.Logs) error {
	di, ordinary := splitDIEvents(input)
	var diErr, logsErr error
	if di.LogRecordCount() > 0 {
		if e.di == nil {
			diErr = consumererror.NewPermanent(errors.New("DI events require datadog.dynamic_instrumentation.enabled; refusing log indexing"))
		} else {
			diErr = e.di.ConsumeDIEvents(ctx, di)
		}
	}
	if ordinary.LogRecordCount() > 0 {
		logsErr = e.Logs.ConsumeLogs(ctx, ordinary)
	}
	return errors.Join(diErr, logsErr)
}

func splitDIEvents(input plog.Logs) (plog.Logs, plog.Logs) {
	di, ordinary := plog.NewLogs(), plog.NewLogs()
	for i := 0; i < input.ResourceLogs().Len(); i++ {
		rl := input.ResourceLogs().At(i)
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				record := sl.LogRecords().At(k)
				_, payload := record.Attributes().Get("datadog.di.payload")
				_, schema := record.Attributes().Get("datadog.di.schema_version")
				dest := ordinary
				if strings.HasPrefix(record.EventName(), "dd.debugger.") || sl.Scope().Name() == "com.datadoghq.dynamic_instrumentation" || payload || schema {
					dest = di
				}
				out := dest.ResourceLogs().AppendEmpty()
				rl.Resource().CopyTo(out.Resource())
				out.SetSchemaUrl(rl.SchemaUrl())
				scope := out.ScopeLogs().AppendEmpty()
				sl.Scope().CopyTo(scope.Scope())
				scope.SetSchemaUrl(sl.SchemaUrl())
				record.CopyTo(scope.LogRecords().AppendEmpty())
			}
		}
	}
	return di, ordinary
}
