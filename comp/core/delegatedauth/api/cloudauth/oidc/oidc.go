// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package oidc adapts workload token sources to the ETS custom OIDC protocol.
package oidc

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// TokenSource obtains a fresh workload JWT for the requested audience. Sources
// must honor cancellation and be safe for concurrent use. They do not exchange
// tokens, discover issuers, or validate signatures; ETS owns those operations.
// Errors must not contain credentials or raw responses.
type TokenSource interface {
	Token(ctx context.Context, audience string) (string, error)
}

// Auth is a provider-independent adapter. The existing delegatedauth component
// owns API key exchange, retries, refresh scheduling, and config writeback.
type Auth struct {
	source        TokenSource
	audience      string
	name          string
	orgUUIDSource string
}

var (
	_          common.Provider                 = (*Auth)(nil)
	_          common.CredentialSourceReporter = (*Auth)(nil)
	orgPattern                                 = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	jwtPattern                                 = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
)

const maxTokenSize = 64 * 1024

// New adapts a custom source without requiring changes to the exchange client.
// orgUUIDSource defaults to "request": send the configured destination org
// alongside the token. ETS authorizes the identity in that org. "claims" keeps
// the legacy wire format for ETS deployments that do not accept explicit routing.
func New(source TokenSource, audience, name, orgUUIDSource string) (*Auth, error) {
	if source == nil {
		return nil, errors.New("OIDC token source is required")
	}
	if orgUUIDSource == "" {
		orgUUIDSource = "request"
	}
	if orgUUIDSource != "claims" && orgUUIDSource != "request" {
		return nil, errors.New("OIDC org_uuid_source must be claims or request")
	}
	return &Auth{source: source, audience: audience, name: name, orgUUIDSource: orgUUIDSource}, nil
}

// LastCredentialSource identifies the configured token source without secrets.
func (a *Auth) LastCredentialSource() string { return "OIDC/" + a.name }

// GenerateAuthProof obtains a token on every exchange, including retries. File
// sources therefore observe kubelet/sidecar rotation without an Agent restart.
func (a *Auth) GenerateAuthProof(ctx context.Context, _ pkgconfigmodel.Reader, config *common.AuthConfig) (string, error) {
	if config == nil || !orgPattern.MatchString(config.OrgUUID) {
		return "", errors.New("OIDC requires a valid org UUID")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	audience := a.audience
	if audience == "" {
		audience = "datadog/" + config.OrgUUID
	}
	token, err := a.source.Token(ctx, audience)
	if err != nil {
		return "", fmt.Errorf("OIDC token acquisition failed: %w", err)
	}
	token = strings.TrimSpace(token)
	// Shape validation only, never authentication. In particular, do not decode
	// claims and mistake them for verified identity or expiration information.
	if len(token) > maxTokenSize || !jwtPattern.MatchString(token) {
		return "", errors.New("OIDC source must return a compact signed JWT no larger than 64 KiB")
	}
	proof := "oidc-" + token
	if a.orgUUIDSource == "request" {
		proof += ":" + config.OrgUUID
	}
	return proof, nil
}
