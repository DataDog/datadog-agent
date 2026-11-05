// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/regions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// MatchesWIFConfig reports whether the identity was enrolled for the configured
// WIF organization and site. Missing enrollment metadata is not a match.
func (identity *PersistedIdentity) MatchesWIFConfig(cfg configmodel.Reader) bool {
	return identity.WIFOrgUUID != "" &&
		strings.EqualFold(identity.WIFOrgUUID, cfg.GetString("delegated_auth.org_uuid")) &&
		identity.WIFSite == parconfig.Site(cfg)
}

// EnrollWithWorkloadToken enrolls a new runner using its primary intake WIF
// identity. Like API-key enrollment, the caller persists the key only on success.
// It makes one attempt, without API-key fallback or existing-runner migration.
func EnrollWithWorkloadToken(ctx context.Context, cfg configmodel.Reader, auth delegatedauth.Component, agentIdentifier *AgentIdentifier) (*Result, error) {
	agentFlavor := flavor.GetFlavor()
	if agentFlavor != flavor.DefaultAgent && agentFlavor != flavor.PrivateActionRunner && agentFlavor != flavor.ClusterAgent {
		return nil, errors.New("WIF enrollment is only supported for node Agent and Cluster Agent runners")
	}
	if auth == nil {
		return nil, errors.New("WIF enrollment requires the delegated-auth component")
	}
	if agentFlavor == flavor.ClusterAgent && agentIdentifier.OrchClusterID == "" {
		return nil, errors.New("WIF enrollment requires the orchestrator cluster ID for Cluster Agent runners")
	}
	wifOrgUUID := cfg.GetString("delegated_auth.org_uuid")
	ddSite := parconfig.Site(cfg)
	runnerNamePrefix := getRunnerNamePrefix(ctx, agentIdentifier)
	privateJWK, publicJWK, err := util.GenerateKeys()
	if err != nil {
		return nil, err
	}
	publicPEM, err := util.JWKToPEM(publicJWK)
	if err != nil {
		return nil, err
	}
	// GenerateKeys sets KeyID to the RFC 7638 SHA-256 public-key thumbprint.
	token, err := auth.GetWorkloadToken(ctx, "api_key", common.WorkloadTokenRequest{
		Grants:        []string{"par_enrollment"},
		JWKThumbprint: publicJWK.KeyID,
	})
	if err != nil {
		return nil, fmt.Errorf("request PAR enrollment workload token: %w", err)
	}
	runnerName := runnerNamePrefix + "-" + time.Now().UTC().Format("20060102150405")
	// Replicas share the cluster identity; it is not tied to a particular host.
	enrollmentHostname := agentIdentifier.Hostname
	if agentFlavor == flavor.ClusterAgent {
		enrollmentHostname = ""
	}
	endpoint, err := parconfig.APIBaseURL(cfg, ddSite)
	if err != nil {
		return nil, err
	}
	client := opms.NewPublicClient(cfg, endpoint, nil)
	response, err := client.EnrollWithWorkloadToken(ctx, token, par.CreateRunnerRequest{
		RunnerName:    runnerName,
		RunnerModes:   []modes.Mode{modes.ModePull},
		PublicKeyPEM:  publicPEM,
		AgentHostname: enrollmentHostname,
		OrchClusterID: agentIdentifier.OrchClusterID,
		AgentFlavor:   agentFlavor,
	})
	if err != nil {
		return nil, err
	}
	return &Result{
		PrivateKey:    privateJWK.Key.(*ecdsa.PrivateKey),
		URN:           util.MakeRunnerURN(regions.GetRegionFromDDSite(ddSite), response.OrgID, response.RunnerID),
		Hostname:      enrollmentHostname,
		OrchClusterID: agentIdentifier.OrchClusterID,
		RunnerName:    runnerName,
		WIFOrgUUID:    wifOrgUUID,
		WIFSite:       ddSite,
		// No APIKeyHash: this identity is authorized by WIF, not the rotating intake key.
	}, nil
}
