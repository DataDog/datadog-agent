// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// ResolveAction runs the side-effect-free Remote Queries target resolution: the
// Agent re-runs the same integration matcher execute uses and answers the
// structured zero/one/many outcome, with the opaque match fingerprint for the
// unique case. Resolve never dispatches a query, never creates a page writer,
// and never touches upload credentials — by construction of the input contract
// and of this mapping.
type ResolveAction struct {
	newBridgeClient BridgeClientFactory
}

func NewResolveAction(newBridgeClient BridgeClientFactory) *ResolveAction {
	return &ResolveAction{newBridgeClient: newBridgeClient}
}

// ResolveInputs is the AP action input injected by the backend: the integration
// and target only. Decoding is strict — any execute-only field (query,
// includeSchema, resultDelivery, matchFingerprint, or anything else) is rejected
// before the bridge client is ever created, so the resolve action cannot carry
// SQL, upload instructions, or credentials even if a malformed task tried to.
type ResolveInputs struct {
	Integration string       `json:"integration"`
	Target      TargetInputs `json:"target"`
}

func (r *ResolveInputs) UnmarshalJSON(data []byte) error {
	var wire struct {
		Integration string       `json:"integration"`
		Target      TargetInputs `json:"target"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	*r = ResolveInputs{Integration: wire.Integration, Target: wire.Target}
	return nil
}

func (a *ResolveAction) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[ResolveInputs](task)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			errors.New("invalid remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	if err := validateTargetInputs(inputs.Target); err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			errors.New("invalid remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	if a == nil || a.newBridgeClient == nil {
		return nil, util.DefaultActionError(errors.New("remote query action requires an Agent IPC client"))
	}
	client, err := a.newBridgeClient()
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query action could not create an Agent IPC client")
	}
	if client == nil {
		return nil, util.DefaultActionError(errors.New("remote query action requires an AgentSecure client"))
	}

	resp, err := client.RemoteQueryResolve(ctx, remoteQueryResolveRequestFromInputs(inputs))
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure resolve RPC failed")
	}
	output, err := remoteQueryResolveOutputFromResponse(resp)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure resolve RPC response was invalid")
	}
	return output, nil
}

// remoteQueryResolveRequestFromInputs maps the AP action input to the
// credential-free AgentSecure resolve request: exactly the integration and the
// target, mirroring execute's target mapping.
func remoteQueryResolveRequestFromInputs(inputs ResolveInputs) *pb.RemoteQueryResolveRequest {
	return &pb.RemoteQueryResolveRequest{
		Integration: inputs.Integration,
		Target: &pb.RemoteQueryTarget{
			Host:             inputs.Target.Host,
			Port:             int32(inputs.Target.Port),
			Dbname:           inputs.Target.DBName,
			DatabaseInstance: inputs.Target.DatabaseInstance,
		},
	}
}

// remoteQueryResolveOutputFromResponse maps the typed AgentSecure resolve response
// to the AP action output, mirroring execute's result-object conventions: exactly
// {status}, plus matchFingerprint when the response carries one and the error
// object when the response carries one. The mapping is deliberately opaque — an
// unknown status or a matched response without a fingerprint passes through
// unchanged so the dispatcher classifies well-formed contract violations itself
// instead of the bundle collapsing them into a transport failure. Only a
// structurally missing response or status fails closed.
func remoteQueryResolveOutputFromResponse(resp *pb.RemoteQueryResolveResponse) (map[string]interface{}, error) {
	if resp == nil {
		return nil, errors.New("remote query resolve response missing")
	}
	status := resp.GetStatus()
	if status == "" {
		return nil, errors.New("remote query resolve response missing status")
	}
	output := map[string]interface{}{"status": status}
	if fingerprint := resp.GetMatchFingerprint(); fingerprint != "" {
		output["matchFingerprint"] = fingerprint
	}
	if code := resp.GetErrorCode(); code != "" {
		output["error"] = map[string]interface{}{
			"code":    code,
			"message": resp.GetErrorMessage(),
		}
	}
	return output, nil
}
