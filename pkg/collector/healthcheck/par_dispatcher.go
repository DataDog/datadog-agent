// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package healthcheck

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	corepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const remediationTimeout = 30 * time.Second

// RegistryDispatcher routes agent-authored remediation through the Remote Agent Registry.
type RegistryDispatcher struct {
	fallback *EventDispatcher
	registry remoteagentregistry.Component
}

// NewRemediationDispatcher selects local execution behind the operator's remediation flag.
func NewRemediationDispatcher(config configmodel.Reader, out chan<- event.Event, hostname string, registry option.Option[remoteagentregistry.Component]) RemediationDispatcher {
	if !config.GetBool("health_check_remediation.enabled") {
		return nil
	}
	// Demo-only: run steps in-process on the host instead of routing through the PAR rshell sandbox.
	if config.GetString("health_check_remediation.execution_mode") == "local" {
		return NewLocalExecDispatcher(out, hostname)
	}
	component, _ := registry.Get()
	return &RegistryDispatcher{
		fallback: NewEventDispatcher(out, hostname),
		registry: component,
	}
}

// Dispatch routes remediation and falls back to dry-run only when non-execution is established.
func (d *RegistryDispatcher) Dispatch(ctx context.Context, id checkid.ID, scName, failureMessage string, cfg *integration.HealthCheckConfig) {
	if d == nil || cfg == nil || ctx.Err() != nil {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, remediationTimeout)
	defer cancel()
	if d.registry == nil {
		d.fallback.Dispatch(runCtx, id, scName, failureMessage, cfg)
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
	response, exitCode, err := d.execute(runCtx, request)
	code := status.Code(err)
	if code == codes.NotFound || code == codes.Unimplemented {
		d.fallback.Dispatch(runCtx, id, scName, failureMessage, cfg)
		return
	}
	d.emit(ctx, id, scName, "detected", detectedText(scName, d.fallback.hostname, failureMessage, id, cfg))
	if err != nil {
		d.emit(ctx, id, scName, "escalate", "Local remediation did not return a confirmed outcome; steps may have executed.")
		return
	}
	outcome := "remediated"
	if exitCode != 0 {
		outcome = "escalate"
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Remediation results for service check %q (check %s):", scName, id)
	for i, step := range response.GetSteps() {
		result := "succeeded"
		if step == nil || step.GetError() != "" || step.GetExitCode() != 0 {
			result = "failed"
			outcome = "escalate"
		}
		command := ""
		if i < len(cfg.Remediation.Steps) {
			command = cfg.Remediation.Steps[i].Command
		}
		fmt.Fprintf(&text, "\n%d. %s -> %s (exit code %d)", i+1, scrubCommand(command), result, step.GetExitCode())
	}
	if len(response.GetSteps()) != len(request.Commands) {
		outcome = "escalate"
		text.WriteString("\nThe full sequence did not complete.")
	}
	d.emit(ctx, id, scName, outcome, text.String())
}

func (d *RegistryDispatcher) execute(ctx context.Context, request *pb.RunLocalRemediationRequest) (*pb.RunLocalRemediationResponse, int32, error) {
	payload, err := proto.Marshal(request)
	if err != nil {
		return nil, 0, err
	}
	command := &corepb.ExecuteCommandRequest{
		ProviderName: "remediation",
		Arguments: &structpb.Struct{Fields: map[string]*structpb.Value{
			"request": structpb.NewStringValue(base64.StdEncoding.EncodeToString(payload)),
		}},
	}
	var binaryOutput []byte
	var exitCode int32
	var receivedOutput, receivedExit bool
	err = d.registry.ExecuteCommand(ctx, command, func(frame *corepb.ExecuteCommandResponse) error {
		if receivedExit {
			return errors.New("remediation output received after exit code")
		}
		switch value := frame.GetFrame().(type) {
		case *corepb.ExecuteCommandResponse_BinaryOutput:
			if receivedOutput {
				return errors.New("multiple remediation response frames")
			}
			binaryOutput = value.BinaryOutput
			receivedOutput = true
		case *corepb.ExecuteCommandResponse_ExitCode:
			if !receivedOutput {
				return errors.New("remediation exit code received before response")
			}
			exitCode = value.ExitCode
			receivedExit = true
		default:
			return errors.New("unexpected remediation output frame")
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if !receivedOutput || !receivedExit {
		return nil, 0, errors.New("incomplete remediation response")
	}
	response := &pb.RunLocalRemediationResponse{}
	if err := proto.Unmarshal(binaryOutput, response); err != nil {
		return nil, 0, err
	}
	return response, exitCode, nil
}

func (d *RegistryDispatcher) emit(ctx context.Context, id checkid.ID, scName, outcome, text string) {
	if d.fallback == nil || d.fallback.out == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	alertType := outcomeAlertType(outcome)
	e := event.Event{
		Title: "health-check remediation (" + outcome + ")", Text: text,
		Ts: time.Now().Unix(), Host: d.fallback.hostname,
		Priority: event.PriorityNormal, AlertType: alertType, SourceTypeName: remediationSource,
		AggregationKey: "health_check_remediation:" + string(id),
		Tags:           []string{"check_id:" + string(id), "service_check:" + scName, "remediation:" + outcome},
	}
	select {
	case d.fallback.out <- e:
	case <-ctx.Done():
	}
}
