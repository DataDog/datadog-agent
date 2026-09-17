// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/regions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

const WorkloadIdentityAuthorization = "workload_identity"

type WorkloadAuthorizer interface {
	GetWorkloadAuthorization(context.Context, string, string) (*common.WorkloadAuthorization, error)
}

// EnrollWorkloadIdentity saves the pending key before making any network call.
// Every retry (including after restart/leader change) reuses that key.
func EnrollWorkloadIdentity(ctx context.Context, cfg configModel.Reader, identifier *AgentIdentifier, authorizer WorkloadAuthorizer) (*Result, error) {
	if authorizer == nil {
		return nil, common.ErrWorkloadAuthorizationUnavailable
	}
	backoff := time.Second
	for {
		identity, err := GetIdentityFromPreviousEnrollment(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if identity != nil && !identity.Pending {
			return resultFromIdentity(identity)
		}
		leader, err := canManageWIFIdentity(cfg)
		if err != nil {
			return nil, err
		}
		if leader {
			if identity == nil {
				key, _, err := util.GenerateKeys()
				if err != nil {
					return nil, err
				}
				candidate := &Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), Hostname: identifier.Hostname, OrchClusterID: identifier.OrchClusterID, RunnerName: identifier.Hostname + "-" + time.Now().UTC().Format("20060102150405"), AuthorizationType: WorkloadIdentityAuthorization, Pending: true}
				if flavor.GetFlavor() == flavor.ClusterAgent {
					candidate.Hostname = ""
				}
				identity, err = claimPendingIdentity(ctx, cfg, candidate)
				if err != nil {
					return nil, err
				}
				if !identity.Pending {
					return resultFromIdentity(identity)
				}
			}
			result, err := exchangeWorkloadIdentity(ctx, cfg, identity, authorizer, false)
			if err == nil {
				if err = PersistIdentity(ctx, cfg, result); err == nil {
					return result, nil
				}
			}
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

// RefreshWorkloadIdentity migrates/re-authorizes only the existing key and runner.
// Followers read the shared identity; only the current leader performs mutations.
func RefreshWorkloadIdentity(ctx context.Context, cfg configModel.Reader, authorizer WorkloadAuthorizer) error {
	if authorizer == nil {
		return common.ErrWorkloadAuthorizationUnavailable
	}
	leader, err := canManageWIFIdentity(cfg)
	if err != nil {
		return err
	}
	if !leader {
		return nil
	}
	identity, err := GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return err
	}
	if identity == nil || identity.Pending {
		return nil
	}
	result, err := exchangeWorkloadIdentity(ctx, cfg, identity, authorizer, true)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return PersistIdentity(ctx, cfg, result)
}

func resultFromIdentity(identity *PersistedIdentity) (*Result, error) {
	key, err := util.Base64ToJWK(identity.PrivateKey)
	if err != nil {
		return nil, errors.New("invalid persisted runner key")
	}
	private, ok := key.Key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("invalid persisted runner key")
	}
	return &Result{AuthorizationVersion: identity.AuthorizationVersion, PrivateKey: private, URN: identity.URN, Hostname: identity.Hostname, OrchClusterID: identity.OrchClusterID, RunnerName: identity.RunnerName, APIKeyHash: identity.APIKeyHash, AuthorizationType: identity.AuthorizationType, IntakeMappingID: identity.IntakeMappingID, Provider: identity.Provider, Pending: identity.Pending}, nil
}

func exchangeWorkloadIdentity(ctx context.Context, cfg configModel.Reader, identity *PersistedIdentity, authorizer WorkloadAuthorizer, reauthorize bool) (*Result, error) {
	result, err := resultFromIdentity(identity)
	if err != nil {
		return nil, err
	}
	public, err := util.EcdsaToJWK(&result.PrivateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	thumbprint, err := public.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, err
	}
	assertion, err := authorizer.GetWorkloadAuthorization(ctx, "api_key", base64.RawURLEncoding.EncodeToString(thumbprint))
	if err != nil {
		return nil, err
	}
	if assertion == nil {
		return nil, common.ErrWorkloadAuthorizationUnavailable
	}
	runnerID, proof := "", ""
	if reauthorize {
		urn, err := util.ParseRunnerURN(identity.URN)
		if err != nil {
			return nil, errors.New("invalid saved runner identity")
		}
		if uint64(urn.OrgID) != assertion.OrgID {
			return nil, errors.New("workload organization does not match runner")
		}
		if identity.AuthorizationType == WorkloadIdentityAuthorization && identity.IntakeMappingID == assertion.IntakeMappingID && identity.Provider == assertion.Provider {
			return nil, nil
		}
		runnerID = urn.RunnerID
		hash := sha256.Sum256([]byte(assertion.Token))
		proof, err = util.GeneratePARJWT(urn.OrgID, runnerID, result.PrivateKey, map[string]any{"expected_authorization_version": identity.AuthorizationVersion, "purpose": "private_action_runner_reauthorization", "assertion_hash": base64.RawURLEncoding.EncodeToString(hash[:])})
		if err != nil {
			return nil, errors.New("failed to sign runner proof")
		}
	}
	site := configutils.ExtractSiteFromURL(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"))
	if site == "" {
		site = "datadoghq.com"
	}
	pem, err := util.JWKToPEM(public)
	if err != nil {
		return nil, err
	}
	response, err := opms.ExchangeWorkloadIdentity(ctx, cfg, enrollmentBaseURL(cfg, site), assertion.Token, runnerID, proof, identity.AuthorizationVersion, &par.CreateRunnerRequest{RunnerName: result.RunnerName, RunnerModes: []modes.Mode{modes.ModePull}, PublicKeyPEM: pem, AgentHostname: result.Hostname, OrchClusterID: result.OrchClusterID, AgentFlavor: flavor.GetFlavor()}, cfg.GetStringMapString(setup.PAROpmsExtraHeaders))
	if err != nil {
		return nil, err
	}
	if uint64(response.OrgID) != assertion.OrgID {
		return nil, errors.New("workload enrollment organization mismatch")
	}
	if !reauthorize {
		result.URN = util.MakeRunnerURN(regions.GetRegionFromDDSite(site), response.OrgID, response.RunnerID)
	}
	result.AuthorizationVersion = response.AuthorizationVersion
	result.AuthorizationType = WorkloadIdentityAuthorization
	result.IntakeMappingID = assertion.IntakeMappingID
	result.Provider = assertion.Provider
	result.APIKeyHash = ""
	result.Pending = false
	return result, nil
}

func WIFIdentityEnabled(cfg configModel.Reader, identity *PersistedIdentity) bool {
	return cfg.GetBool(setup.PARWorkloadIdentityEnrollment) || (identity != nil && identity.AuthorizationType == WorkloadIdentityAuthorization)
}

// ValidateWorkloadIdentityHost prevents treating a moved identity as a new runner.
func ValidateWorkloadIdentityHost(identity *PersistedIdentity, identifier *AgentIdentifier) error {
	if identity != nil && identity.Hostname != "" && identity.Hostname != identifier.Hostname {
		return fmt.Errorf("saved workload runner belongs to another hostname")
	}
	return nil
}
