// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package healthcheck

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

const executorProbeTimeout = time.Second
const remediationTimeout = 30 * time.Second

// PARDispatcher sends agent-authored remediation to the authenticated local executor.
type PARDispatcher struct {
	fallback  *EventDispatcher
	address   string
	tlsConfig func() (*tls.Config, error)
}

// NewRemediationDispatcher selects local execution behind the operator's remediation flag.
func NewRemediationDispatcher(config configmodel.Reader, out chan<- event.Event, hostname string) RemediationDispatcher {
	if !config.GetBool("health_check_remediation.enabled") {
		return nil
	}
	return &PARDispatcher{
		fallback: NewEventDispatcher(out, hostname),
		address:  config.GetString(privateactionrunner.PARExecutorSocketPath),
		tlsConfig: func() (*tls.Config, error) {
			client, server, _, err := cert.FetchIPCCert(config)
			if err != nil {
				return nil, err
			}
			if len(server.Certificates) == 0 {
				return nil, errors.New("shared IPC certificate is missing")
			}
			client = client.Clone()
			client.Certificates = server.Certificates
			client.ServerName = "localhost"
			return client, nil
		},
	}
}

// Dispatch falls back before execution when PAR is unavailable and reports uncertain RPC outcomes as failures.
func (d *PARDispatcher) Dispatch(ctx context.Context, id checkid.ID, scName string, cfg *integration.HealthCheckConfig) {
	if d == nil || cfg == nil || ctx.Err() != nil {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, remediationTimeout)
	defer cancel()
	connection, err := d.connect()
	if err != nil {
		d.fallback.Dispatch(runCtx, id, scName, cfg)
		return
	}
	defer connection.Close()
	client := pb.NewExecutorClient(connection)
	probeCtx, probeCancel := context.WithTimeout(runCtx, executorProbeTimeout)
	health, err := client.Health(probeCtx, &pb.HealthRequest{})
	probeCancel()
	if err != nil || !health.GetReady() {
		d.fallback.Dispatch(runCtx, id, scName, cfg)
		return
	}
	policy, err := synthesizeAllowlist(cfg.Remediation)
	if err != nil {
		d.emit(ctx, id, scName, "escalate", "Local remediation policy is invalid; no steps were executed.")
		return
	}
	request := &pb.RunLocalRemediationRequest{Allowlist: policy}
	for _, step := range cfg.Remediation.Steps {
		request.Commands = append(request.Commands, step.Command)
	}
	response, err := client.RunLocalRemediation(runCtx, request)
	if status.Code(err) == codes.Unimplemented {
		d.fallback.Dispatch(runCtx, id, scName, cfg)
		return
	}
	d.emit(ctx, id, scName, "detected", "Health check became CRITICAL; local remediation was dispatched.")
	if err != nil {
		d.emit(ctx, id, scName, "escalate", "Local remediation did not return a confirmed outcome; steps may have executed.")
		return
	}
	outcome := "remediated"
	var text strings.Builder
	text.WriteString("Local remediation step results:")
	for i, step := range response.GetSteps() {
		result := "succeeded"
		if step == nil || step.GetError() != "" || step.GetExitCode() != 0 {
			result = "failed"
			outcome = "escalate"
		}
		fmt.Fprintf(&text, "\n%d. %s (exit code %d)", i+1, result, step.GetExitCode())
	}
	if len(response.GetSteps()) != len(request.Commands) {
		outcome = "escalate"
		text.WriteString("\nThe full sequence did not complete.")
	}
	d.emit(ctx, id, scName, outcome, text.String())
}

func (d *PARDispatcher) connect() (*grpc.ClientConn, error) {
	if d.address == "" {
		return nil, errors.New("local executor address is missing")
	}
	tlsConfig, err := d.tlsConfig()
	if err != nil {
		return nil, err
	}
	return grpc.NewClient("passthrough:///localhost", grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return executor.Dial(ctx, d.address, executorProbeTimeout)
		}))
}

func (d *PARDispatcher) emit(ctx context.Context, id checkid.ID, scName, outcome, text string) {
	if d.fallback == nil || d.fallback.out == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	alertType := event.AlertTypeInfo
	if outcome == "escalate" {
		alertType = event.AlertTypeError
	}
	e := event.Event{
		Title: "health-check remediation (" + outcome + ")", Text: text,
		Ts: time.Now().Unix(), Host: d.fallback.hostname,
		Priority: event.PriorityNormal, AlertType: alertType, SourceTypeName: "datadog-agent",
		AggregationKey: "health_check_remediation:" + string(id),
		Tags:           []string{"check_id:" + string(id), "service_check:" + scName, "remediation:" + outcome},
	}
	select {
	case d.fallback.out <- e:
	case <-ctx.Done():
	}
}
