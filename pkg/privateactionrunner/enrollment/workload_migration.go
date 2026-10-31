// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// MigrateToWIF authorizes the saved node runner through its primary intake WIF
// mapping. It keeps the existing key and URN; the caller persists only on success.
// A later attempt uses the same identity and a fresh workload token and proof.
func MigrateToWIF(ctx context.Context, cfg configmodel.Reader, auth delegatedauth.Component, agentIdentifier *AgentIdentifier, identity *PersistedIdentity) (*Result, error) {
	if flavor.GetFlavor() != flavor.DefaultAgent && flavor.GetFlavor() != flavor.PrivateActionRunner {
		return nil, errors.New("WIF migration is only supported for node Agent runners")
	}
	if auth == nil {
		return nil, errors.New("WIF migration requires the delegated-auth component")
	}
	if identity == nil || agentIdentifier == nil {
		return nil, errors.New("WIF migration requires a saved runner identity and hostname")
	}
	if identity.Hostname != "" && identity.Hostname != agentIdentifier.Hostname {
		return nil, errors.New("WIF migration cannot use an identity saved for a different hostname")
	}
	parts, err := util.ParseRunnerURN(identity.URN)
	if err != nil || parts.OrgID <= 0 || parts.RunnerID == "" {
		return nil, errors.New("WIF migration requires a valid saved runner URN")
	}
	jwk, err := util.Base64ToJWK(identity.PrivateKey)
	if err != nil {
		return nil, errors.New("WIF migration cannot decode the saved runner key")
	}
	key, ok := jwk.Key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("WIF migration requires the saved runner's ECDSA private key")
	}
	publicJWK, err := util.EcdsaToJWK(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	token, err := auth.GetWorkloadToken(ctx, "api_key", common.WorkloadTokenRequest{
		Grants: []string{"par_enrollment"}, JWKThumbprint: publicJWK.KeyID,
	})
	if err != nil {
		return nil, fmt.Errorf("request PAR migration workload token: %w", err)
	}
	hash := sha256.Sum256([]byte(token))
	proof, err := util.GeneratePARJWT(parts.OrgID, parts.RunnerID, key, map[string]any{
		"aud":                 "private-action-runner-wif-migration",
		"workload_token_hash": base64.RawURLEncoding.EncodeToString(hash[:]),
	})
	if err != nil {
		return nil, err
	}
	ddSite := configutils.ExtractSiteFromURL(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"))
	if ddSite == "" {
		ddSite = "datadoghq.com"
	}
	client := opms.NewPublicClient(cfg, enrollmentBaseURL(cfg, ddSite), nil)
	response, err := client.MigrateWithWorkloadToken(ctx, token, parts.RunnerID, proof)
	if err != nil {
		return nil, err
	}
	if response.RunnerID != parts.RunnerID || response.OrgID != parts.OrgID {
		return nil, errors.New("WIF migration returned a different runner identity")
	}
	return &Result{PrivateKey: key, URN: identity.URN, Hostname: identity.Hostname, WIFEnrolled: true}, nil
}
