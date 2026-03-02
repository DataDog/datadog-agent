// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rawotlpexporter

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.uber.org/zap"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

type tracesExporter struct {
	client pb.RawTraceServiceClient
	logger *zap.Logger
}

func (e *tracesExporter) consumeTraces(ctx context.Context, td ptrace.Traces) error {
	if td.SpanCount() == 0 {
		return nil
	}
	req := ptraceotlp.NewExportRequestFromTraces(td)
	data, err := req.MarshalProto()
	if err != nil {
		e.logger.Debug("rawotlpexporter: failed to marshal ExportTraceServiceRequest", zap.Error(err))
		return err
	}
	_, err = e.client.ExportTracesRaw(ctx, &pb.ExportTracesRawRequest{Data: data})
	if err != nil {
		e.logger.Debug("rawotlpexporter: ExportTracesRaw failed", zap.Error(err))
		return err
	}
	return nil
}
