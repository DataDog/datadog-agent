// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/regions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

const defaultIdentityFileName = "privateactionrunner_private_identity.json"

// Result contains the result of a successful enrollment
type Result struct {
	PrivateKey    *ecdsa.PrivateKey
	URN           string
	Hostname      string
	RunnerName    string
	OrchClusterID string
}

type AgentIdentifier struct {
	Hostname      string
	OrchClusterID string
}

type PersistedIdentity struct {
	PrivateKey    string `json:"private_key"`
	URN           string `json:"urn"`
	Hostname      string `json:"hostname,omitempty"`
	OrchClusterID string `json:"orch_cluster_id,omitempty"`
}

// GetAgentIdentifier returns the identifier for the current agent.
// Hostname is always populated. For the cluster agent, OrchClusterID is also populated (required).
func GetAgentIdentifier(ctx context.Context, hostnameGetter hostnameinterface.Component) (*AgentIdentifier, error) {
	hostname, err := hostnameGetter.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	agentIdentifier := &AgentIdentifier{Hostname: hostname}
	if flavor.GetFlavor() == flavor.ClusterAgent {
		orchClusterID, err := clustername.GetClusterID()
		if err != nil || orchClusterID == "" {
			return nil, fmt.Errorf("failed to get orchestrator cluster ID for cluster agent: %w", err)
		}
		agentIdentifier.OrchClusterID = orchClusterID
	}
	return agentIdentifier, nil
}

// ShouldReenroll checks whether the persisted identity needs refreshing.
// Re-enrollment is only supported for the node agent.
func ShouldReenroll(agentIdentifier *AgentIdentifier, identity *PersistedIdentity) bool {
	if identity == nil || flavor.GetFlavor() == flavor.ClusterAgent {
		return false
	}
	if identity.Hostname != "" && identity.Hostname != agentIdentifier.Hostname {
		log.Infof("Saved identity hostname does not match current hostname, re-enrolling")
		return true
	}
	return false
}

// SelfEnroll performs self-registration of a private action runner using API credentials
func SelfEnroll(
	ctx context.Context,
	ddSite,
	runnerNamePrefix,
	apiKey,
	appKey string,
	agentIdentifier *AgentIdentifier,
	extraHeaders map[string]string,
) (*Result, error) {
	agentFlavor := flavor.GetFlavor()

	now := time.Now().UTC()
	formattedTime := now.Format("20060102150405")
	runnerName := runnerNamePrefix + "-" + formattedTime

	privateJwk, publicJwk, err := util.GenerateKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	ddBaseURL := "https://api." + ddSite
	publicClient := opms.NewPublicClient(ddBaseURL, extraHeaders)

	runnerModes := []modes.Mode{modes.ModePull}

	// The cluster agent is a deployment not tied to a specific host, so hostname is not sent.
	enrollmentHostname := agentIdentifier.Hostname
	if agentFlavor == flavor.ClusterAgent {
		enrollmentHostname = ""
	}

	createRunnerResponse, err := publicClient.EnrollWithApiKey(
		ctx,
		apiKey,
		appKey,
		runnerName,
		runnerModes,
		publicJwk,
		enrollmentHostname,
		agentIdentifier.OrchClusterID,
		agentFlavor,
	)
	if err != nil {
		return nil, fmt.Errorf("enrollment API call failed: %w", err)
	}

	region := regions.GetRegionFromDDSite(ddSite)
	urn := util.MakeRunnerURN(region, createRunnerResponse.OrgID, createRunnerResponse.RunnerID)

	return &Result{
		PrivateKey:    privateJwk.Key.(*ecdsa.PrivateKey),
		URN:           urn,
		Hostname:      enrollmentHostname,
		RunnerName:    runnerName,
		OrchClusterID: agentIdentifier.OrchClusterID,
	}, nil
}
