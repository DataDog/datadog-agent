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
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/regions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// EnrollWithWorkloadToken enrolls a new node runner using its primary intake WIF
// identity. Like API-key enrollment, the caller persists the key only on success.
// It makes one attempt, without API-key fallback or existing-runner migration.
func EnrollWithWorkloadToken(ctx context.Context, cfg configmodel.Reader, auth delegatedauth.Component, agentIdentifier *AgentIdentifier) (*Result, error) {
	if flavor.GetFlavor() != flavor.DefaultAgent && flavor.GetFlavor() != flavor.PrivateActionRunner {
		return nil, errors.New("WIF enrollment is only supported for node Agent runners")
	}
	if auth == nil {
		return nil, errors.New("WIF enrollment requires the delegated-auth component")
	}
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
	ddSite := configutils.ExtractSiteFromURL(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"))
	if ddSite == "" {
		ddSite = "datadoghq.com"
	}
	runnerName := agentIdentifier.Hostname + "-" + time.Now().UTC().Format("20060102150405")
	response, err := opms.EnrollWithWorkloadToken(ctx, cfg, enrollmentBaseURL(cfg, ddSite), token, par.CreateRunnerRequest{
		RunnerName:    runnerName,
		RunnerModes:   []modes.Mode{modes.ModePull},
		PublicKeyPEM:  publicPEM,
		AgentHostname: agentIdentifier.Hostname,
		AgentFlavor:   flavor.GetFlavor(),
	})
	if err != nil {
		return nil, err
	}
	return &Result{
		PrivateKey:  privateJWK.Key.(*ecdsa.PrivateKey),
		URN:         util.MakeRunnerURN(regions.GetRegionFromDDSite(ddSite), response.OrgID, response.RunnerID),
		Hostname:    agentIdentifier.Hostname,
		RunnerName:  runnerName,
		WIFEnrolled: true,
		// No APIKeyHash: this identity is authorized by WIF, not the rotating intake key.
	}, nil
}
