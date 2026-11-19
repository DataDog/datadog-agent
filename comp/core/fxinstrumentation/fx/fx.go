// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the fxinstrumentation component
package fx

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type tracingParams struct {
	fx.In
	Config   config.Component
	Hostname hostnameinterface.Component `optional:"true"`
}

// Module enables startup traces via the local trace-agent and shutdown traces
// directly to the configured APM intake, independently of local process order.
// The tracer is implemented in pkg/util/fxutil/logging/tracer.go.
// Traces are sent only when the environment variable DD_FX_TRACING_ENABLED is set to true and this module added to the Fx application.
func Module() fxutil.Module {
	return fxutil.Component(
		// No need to call ProvideComponentConstructor because this module is not a component.
		fx.Invoke(func(logger fxevent.Logger, params tracingParams) {
			if instrumentedLogger, ok := logger.(*logging.FxTracingLogger); ok {
				instrumentedLogger.EnableSpansSending(params.Config.GetString("apm_config.receiver_port"))
				hostname := params.Config.GetString("hostname")
				if params.Hostname != nil {
					hostname = params.Hostname.GetSafe(context.Background())
				}
				sender := newShutdownSender(params.Config, hostname)
				instrumentedLogger.EnableShutdownTracing(version.AgentVersion, sender.send)
			}
		}),
	)
}

const shutdownExportTimeout = 2 * time.Second

// shutdownSender owns its HTTP transport and reads only cached configuration.
// It never needs the local trace-agent, config API, or a running Fx component.
type shutdownSender struct {
	cfg      model.Reader
	hostname string
	client   *http.Client
}

func newShutdownSender(cfg model.Reader, hostname string) *shutdownSender {
	return &shutdownSender{cfg: cfg, hostname: hostname, client: &http.Client{
		Transport: httputils.CreateHTTPTransport(cfg),
		Timeout:   shutdownExportTimeout,
		// API keys must never be forwarded to a redirect destination.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (s *shutdownSender) send(spans []*logging.Span) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownExportTimeout)
	defer cancel()
	defer s.client.CloseIdleConnections()
	if err := s.export(ctx, spans); err != nil {
		// Do not log errors from transports or endpoints: they can contain credentials.
		fmt.Fprintln(os.Stderr, "[Fx Tracing] Shutdown trace export failed:", err) //nolint:errcheck
	}
}

func (s *shutdownSender) export(ctx context.Context, spans []*logging.Span) error {
	if len(spans) == 0 {
		return nil
	}
	targets, destinationErr := s.destinations()
	if len(targets) == 0 {
		return destinationErr
	}

	tags := make(map[string]string)
	for _, tag := range s.cfg.GetStringSlice("tags") {
		k, v, ok := strings.Cut(tag, ":")
		if ok {
			tags[k] = v
		}
	}
	if cluster := s.cfg.GetString("cluster_name"); cluster != "" {
		tags["kube_cluster_name"] = cluster
	}
	env := s.cfg.GetString("env")
	converted := make([]*pb.Span, 0, len(spans))
	for _, span := range spans {
		meta := maps.Clone(tags)
		maps.Copy(meta, span.Meta)
		if env != "" {
			meta["env"] = env
		}
		metrics := map[string]float64{"_dd.analyzed": 1, "_dd1.sr.eausr": 1, "_sampling_priority_v1": 2}
		if span.ParentID == 0 {
			metrics["_top_level"] = 1
		}
		converted = append(converted, &pb.Span{
			Service: span.Service, Name: span.Name, Resource: span.Resource,
			TraceID: span.TraceID, SpanID: span.SpanID, ParentID: span.ParentID,
			Start: span.Start, Duration: span.Duration, Error: span.Error,
			Meta: meta, Metrics: metrics, Type: span.Type,
		})
	}
	payload := &pb.AgentPayload{
		HostName: s.hostname, Env: env, AgentVersion: spans[len(spans)-1].Meta["agent.version"], Tags: tags,
		TracerPayloads: []*pb.TracerPayload{{
			Hostname: s.hostname, Env: env, LanguageName: "go", LanguageVersion: strings.TrimPrefix(runtime.Version(), "go"),
			Chunks: []*pb.TraceChunk{{Priority: 2, Spans: converted}},
		}},
	}
	data, err := payload.MarshalVT()
	if err != nil {
		return errors.New("cannot encode shutdown trace")
	}
	var body bytes.Buffer
	zw := gzip.NewWriter(&body)
	if _, err := zw.Write(data); err != nil {
		return errors.New("cannot compress shutdown trace")
	}
	if err := zw.Close(); err != nil {
		return errors.New("cannot finish shutdown trace compression")
	}
	// All destinations share one deadline. A slow primary must not consume the
	// entire budget before a secondary (such as ddstaging) is attempted.
	results := make(chan error, len(targets))
	var sends sync.WaitGroup
	for _, target := range targets {
		sends.Go(func() { results <- s.post(ctx, target, body.Bytes()) })
	}
	sends.Wait()
	close(results)
	errs := []error{destinationErr}
	for err := range results {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

type shutdownDestination struct{ url, apiKey string }

func (s *shutdownSender) destinations() ([]shutdownDestination, error) {
	var targets []shutdownDestination
	seen := make(map[shutdownDestination]bool)
	skipped := 0
	add := func(endpoint, key string) {
		key = configutils.SanitizeAPIKey(key)
		if endpoint == "" || key == "" || strings.HasPrefix(key, "ENC[") || configutils.IsDelaDirective(key) {
			skipped++
			return
		}
		target := shutdownDestination{url: strings.TrimRight(endpoint, "/"), apiKey: key}
		if !seen[target] {
			seen[target] = true
			targets = append(targets, target)
		}
	}
	if s.cfg.GetBool("apm_config.traces_send_to_main_endpoint") {
		endpoint := configutils.GetMainEndpoint(s.cfg, "https://trace.agent.", "apm_config.apm_dd_url")
		for _, prefix := range []string{"observability_pipelines_worker", "vector"} {
			if s.cfg.GetBool(prefix + ".traces.enabled") {
				endpoint = s.cfg.GetString(prefix + ".traces.url")
				break
			}
		}
		add(endpoint, s.cfg.GetString("api_key"))
	}
	for endpoint, keys := range s.cfg.GetStringMapStringSlice("apm_config.additional_endpoints") {
		for _, key := range keys {
			add(endpoint, key)
		}
	}
	if len(targets) == 0 {
		return nil, errors.New("no resolved APM destination available")
	}
	if skipped > 0 {
		return targets, fmt.Errorf("%d APM destinations lacked a URL or resolved credentials", skipped)
	}
	return targets, nil
}

func (s *shutdownSender) post(ctx context.Context, target shutdownDestination, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.url+"/api/v0.2/traces", bytes.NewReader(data))
	if err != nil {
		return errors.New("invalid trace intake destination")
	}
	req.Header.Set("DD-API-KEY", target.apiKey)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New("trace intake request failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("trace intake returned HTTP %d", resp.StatusCode)
	}
	return nil
}
