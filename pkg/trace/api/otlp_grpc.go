// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package api

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/api/loader"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// grpcServer is the server accepting OTLP traces forwarded by the core Agent.
type grpcServer = *grpc.Server

var _ (ptraceotlp.GRPCServer) = (*OTLPReceiver)(nil)

// Start starts the OTLPReceiver, if any of the servers were configured as active.
func (o *OTLPReceiver) Start() {
	cfg := o.conf.OTLPReceiver
	if cfg.GRPCPort != 0 {
		var ln net.Listener
		var err error
		// When using the trace-loader, the OTLP listener might be provided as an already opened file descriptor
		// so we try to get a listener from it, and fallback to listening on the given address if it fails
		if grpcFDStr, ok := os.LookupEnv("DD_OTLP_CONFIG_GRPC_FD"); ok {
			ln, err = loader.GetListenerFromFD(grpcFDStr, "otlp_conn")
			if err == nil {
				log.Debugf("Using OTLP listener from file descriptor %s", grpcFDStr)
			} else {
				log.Errorf("Error creating OTLP listener from file descriptor %s: %v", grpcFDStr, err)
			}
		}
		if ln == nil {
			// if the fd was not provided, or we failed to get a listener from it, listen on the given address
			ln, err = loader.GetTCPListener(net.JoinHostPort(cfg.BindHost, strconv.Itoa(cfg.GRPCPort)))
		}

		if err != nil {
			log.Criticalf("Error starting OpenTelemetry gRPC server: %v", err)
		} else {
			opts := []grpc.ServerOption{
				grpc.MaxRecvMsgSize(o.grpcMaxRecvMsgSize),
				grpc.MaxConcurrentStreams(1), // Each payload must be sent to processing stage before we decode the next.
			}

			// OTLP trace ingestion doesn't need generic gRPC metrics interceptors
			// since we collect business-specific metrics (spans, traces, payloads) via StatsD

			o.grpcsrv = grpc.NewServer(opts...)
			ptraceotlp.RegisterGRPCServer(o.grpcsrv, o)
			o.wg.Add(1)
			go func() {
				defer o.wg.Done()
				if err := o.grpcsrv.Serve(ln); err != nil {
					log.Criticalf("Error starting OpenTelemetry gRPC server: %v", err)
				}
			}()
			log.Debugf("Listening to core Agent for OTLP traces on internal gRPC port (http://%s:%d, internal use only). Check core Agent logs for information on the OTLP ingest status.", cfg.BindHost, cfg.GRPCPort)
		}
	}
}

func (o *OTLPReceiver) stopGRPC() {
	if o.grpcsrv != nil {
		go o.grpcsrv.Stop()
	}
}

// Export implements ptraceotlp.Server
func (o *OTLPReceiver) Export(ctx context.Context, in ptraceotlp.ExportRequest) (ptraceotlp.ExportResponse, error) {
	defer o.timing.Since("datadog.trace_agent.otlp.process_grpc_request_ms", time.Now())
	md, _ := metadata.FromIncomingContext(ctx)
	_ = o.statsd.Count("datadog.trace_agent.otlp.payload", 1, tagsFromHeaders(http.Header(md)), 1)
	o.processRequest(ctx, http.Header(md), in)
	return ptraceotlp.NewExportResponse(), nil
}

func tagsFromHeaders(h http.Header) []string {
	tags := []string{"endpoint_version:opentelemetry_grpc_v1"}
	if v := fastHeaderGet(h, header.Lang); v != "" {
		tags = append(tags, "lang:"+v)
	}
	if v := fastHeaderGet(h, header.LangVersion); v != "" {
		tags = append(tags, "lang_version:"+v)
	}
	if v := fastHeaderGet(h, header.LangInterpreter); v != "" {
		tags = append(tags, "interpreter:"+v)
	}
	if v := fastHeaderGet(h, header.LangInterpreterVendor); v != "" {
		tags = append(tags, "lang_vendor:"+v)
	}
	return tags
}
