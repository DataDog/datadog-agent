// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package bootstrapparcontrol

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	par "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type enrollAndPersistFunc func(context.Context, log.Component, config.Component, *enrollment.AgentIdentifier) (*enrollment.Result, error)

// ensureEnrollment guarantees that a usable runner identity exists, reusing a
// persisted identity when it belongs to the current Agent host and falling back
// to self-enrollment.
func ensureEnrollment(ctx context.Context, logger log.Component, cfg config.Component, hostnameComp hostname.Component, enrollAndPersist enrollAndPersistFunc) error {
	agentIdentifier, err := enrollment.GetAgentIdentifier(ctx, hostnameComp)
	if err != nil {
		return fmt.Errorf("failed to get agent identifier: %w", err)
	}

	discardPersisted := false

	persisted, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	switch {
	case err != nil && !errors.Is(err, enrollment.ErrIdentityCorrupt):
		// May be transient; re-enrolling would register a second runner.
		return fmt.Errorf("failed to load persisted identity: %w", err)
	case err != nil:
		logger.Warnf("Discarding unusable persisted identity: %v", err)
		discardPersisted = true
	case persisted != nil:
		if err := validateIdentity(persisted.URN, persisted.PrivateKey); err != nil {
			logger.Warnf("Discarding invalid persisted identity: %v", err)
			discardPersisted = true
		} else if !enrollment.ShouldReenroll(agentIdentifier, persisted) {
			logger.Info("Persisted identity is valid; enrollment is not required")
			return nil
		} else {
			discardPersisted = true
		}
	}

	configuredURN := cfg.GetString(par.PARUrn)
	configuredPrivateKey := cfg.GetString(par.PARPrivateKey)
	if err := validateConfiguredIdentity(configuredURN, configuredPrivateKey); err != nil {
		return err
	}
	if configuredURN != "" && configuredPrivateKey != "" {
		// A persisted identity wins over inline configuration, so an unusable
		// file has to go before the inline identity can be used.
		if discardPersisted {
			if err := enrollment.RemoveIdentityFile(cfg); err != nil {
				return err
			}
		}
		logger.Info("Configured identity is complete; enrollment is not required")
		return nil
	}

	if !cfg.GetBool(par.PARSelfEnroll) {
		return errors.New("no valid Private Action Runner identity is available and private_action_runner.self_enroll is false; configure a URN and private key or enable self-enrollment")
	}

	result, err := enrollAndPersist(ctx, logger, cfg, agentIdentifier)
	if err != nil {
		return err
	}
	logger.Infof("Identity successfully enrolled. New URN: %s", result.URN)
	return nil
}

func validateConfiguredIdentity(urn, privateKey string) error {
	if urn != "" {
		if _, err := parutil.ParseRunnerURN(urn); err != nil {
			return fmt.Errorf("configured private_action_runner.urn is invalid: %w", err)
		}
	}
	if privateKey != "" {
		if err := validatePrivateKey(privateKey); err != nil {
			return fmt.Errorf("configured private_action_runner.private_key is invalid: %w", err)
		}
	}
	return nil
}

func validateIdentity(urn, privateKey string) error {
	if _, err := parutil.ParseRunnerURN(urn); err != nil {
		return fmt.Errorf("invalid URN: %w", err)
	}
	if err := validatePrivateKey(privateKey); err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}
	return nil
}

func validatePrivateKey(encoded string) error {
	jwk, err := parutil.Base64ToJWK(encoded)
	if err != nil {
		return err
	}
	if _, ok := jwk.Key.(*ecdsa.PrivateKey); !ok {
		return errors.New("JWK does not contain an ECDSA private key")
	}
	return nil
}
