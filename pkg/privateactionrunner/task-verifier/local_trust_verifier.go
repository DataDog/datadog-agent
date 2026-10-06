// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"errors"
	"maps"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

// LocalTrustVerifier trusts on-host policy exclusively for unsigned, non-privileged remediation.
type LocalTrustVerifier struct {
	orgID    int64
	runnerID string
}

// NewLocalTrustVerifier is an explicit local trust boundary; signed workflows retain NewTaskVerifier.
func NewLocalTrustVerifier(cfg *config.Config) TaskVerifier {
	return &LocalTrustVerifier{orgID: cfg.OrgId, runnerID: cfg.RunnerId}
}

// UnwrapTask substitutes operator-authored local policy for signed policy without granting escalation.
func (v *LocalTrustVerifier) UnwrapTask(task *types.Task) (*types.Task, error) {
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("local remediation task is required")
	}
	attrs := task.Data.Attributes
	if attrs.BundleID != "com.datadoghq.remoteaction.rshell" || attrs.Name != "runRemediationCommand" {
		return nil, errors.New("local verifier only accepts rshell remediation tasks")
	}
	if attrs.SignedEnvelope != nil || attrs.VerificationKey != nil {
		return nil, errors.New("local remediation cannot carry signed authority")
	}
	connection := attrs.ConnectionInfo
	if connection.GetConnectionId() != "" || len(connection.GetTokens()) != 0 || connection.GetCredentialsType() != privateactionspb.CredentialsType_UNSPECIFIED || attrs.SecDatadogHeaderValue != "" {
		return nil, errors.New("local remediation cannot use credentials")
	}
	for key, value := range attrs.Inputs {
		switch key {
		case "command", "timeout":
		case "effectivePermissions":
			if permissions, ok := value.(string); !ok || permissions != "" {
				return nil, errors.New("local remediation cannot grant escalation")
			}
		case "elevatableCommands":
			switch commands := value.(type) {
			case []string:
				if len(commands) != 0 {
					return nil, errors.New("local remediation cannot grant elevatable commands")
				}
			case []interface{}:
				if len(commands) != 0 {
					return nil, errors.New("local remediation cannot grant elevatable commands")
				}
			default:
				return nil, errors.New("local remediation cannot grant elevatable commands")
			}
		default:
			return nil, errors.New("unsupported local remediation input")
		}
	}
	if command, ok := attrs.Inputs["command"].(string); !ok || strings.TrimSpace(command) == "" {
		return nil, errors.New("local remediation command is required")
	}
	policy := attrs.SystemInputs.GetRemoteAction()
	if err := ValidateLocalRemediationPolicy(policy); err != nil {
		return nil, err
	}
	if policy == nil {
		policy = &privateactionspb.RemoteAction{}
	} else {
		policy = proto.Clone(policy).(*privateactionspb.RemoteAction)
	}
	unwrapped := *task
	unwrapped.Raw = nil
	localAttrs := *attrs
	unwrapped.Data.Attributes = &localAttrs
	localAttrs.Inputs = maps.Clone(attrs.Inputs)
	delete(localAttrs.Inputs, "effectivePermissions")
	delete(localAttrs.Inputs, "elevatableCommands")
	localAttrs.OrgId = v.orgID
	localAttrs.ConnectionInfo = &privateactionspb.ConnectionInfo{RunnerId: v.runnerID}
	localAttrs.SystemInputs = &privateactionspb.SystemInputs{Input: &privateactionspb.SystemInputs_RemoteAction{RemoteAction: policy}}
	return &unwrapped, nil
}

// ValidateLocalRemediationPolicy rejects wildcard grants in the local substitute for signed policy.
func ValidateLocalRemediationPolicy(policy *privateactionspb.RemoteAction) error {
	if policy == nil {
		return nil
	}
	for _, command := range policy.AllowedCommands {
		name, namespaced := strings.CutPrefix(command, "rshell:")
		if !namespaced || name == "" || strings.ContainsAny(name, "*?[]{}:/\\ \t\r\n") {
			return errors.New("local remediation commands must be exact rshell names")
		}
	}
	for _, path := range policy.AllowedPaths {
		if strings.TrimSpace(path) == "" || strings.ContainsAny(path, "*?[]{}") {
			return errors.New("local remediation paths cannot be empty or contain wildcards")
		}
	}
	for service, actions := range policy.SystemServices {
		if strings.TrimSpace(service) == "" || strings.ContainsAny(service, "*?[]{}") {
			return errors.New("local remediation services must be exact names")
		}
		for _, action := range actions.GetValues() {
			value, ok := action.GetKind().(*structpb.Value_StringValue)
			if !ok || strings.TrimSpace(value.StringValue) == "" || strings.ContainsAny(value.StringValue, "*?[]{}") {
				return errors.New("local remediation service actions must be exact strings")
			}
		}
	}
	return nil
}
