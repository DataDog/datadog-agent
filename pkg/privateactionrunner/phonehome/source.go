// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"net/url"
	"os"
	"strings"
	"time"

	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
)

// Prerequisites is a credential-free, value observation. Injected observations
// in controller tests are not evidence that real discovery has run.
type Prerequisites struct {
	RCReady        bool
	RCReason       string
	ScopeReady     bool
	ScopeReason    string
	ScopeCheckedAt time.Time
	NextCheck      time.Time
}

// Source owns observations and discovery demand; reading never forces a request.
type Source interface {
	Read(time.Time) Prerequisites
	Demand(bool)
}

type rcSource struct {
	cfg                model.Reader
	service            rcservice.Component
	scope              ScopeSource
	key, rcURL, parURL string
}

// NewSource combines optional RC status and PAR-owned scope without doing HTTP.
func NewSource(cfg model.Reader, service rcservice.Component, scope ScopeSource) Source {
	return &rcSource{cfg: cfg, service: service, scope: scope, key: cfg.GetString("api_key"), rcURL: configutils.GetMainEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url"), parURL: enrollmentEndpoint(cfg)}
}

func (r *rcSource) Demand(wanted bool) {
	if r.scope != nil {
		r.scope.SetPARDiscoveryDemand(wanted)
	}
}

func (r *rcSource) Read(now time.Time) Prerequisites {
	p := Prerequisites{RCReason: "rc_service_unavailable", ScopeReason: "par_scope_unchecked"}
	if !configutils.IsRemoteConfigEnabled(r.cfg) {
		p.RCReason = "rc_locally_disabled"
		return p
	}
	if r.service == nil {
		return p
	}
	scope := PARScopeObservation{Reason: "par_scope_unchecked"}
	if r.scope != nil {
		scope = r.scope.GetPARScope()
	}
	p.ScopeReason, p.ScopeCheckedAt, p.NextCheck = scope.Reason, scope.CheckedAt, scope.NextCheck
	if p.ScopeReason == "" {
		p.ScopeReason = "par_scope_unchecked"
	}
	rc := r.service.GetOrgStatus()
	if rc == nil {
		p.RCReason = "rc_unchecked"
		return p
	}
	key := r.cfg.GetString("api_key")
	rcURL := configutils.GetMainEndpoint(r.cfg, "https://config.", "remote_configuration.rc_dd_url")
	parURL := enrollmentEndpoint(r.cfg)
	expectedRC := "https://config." + enrollmentSite(r.cfg)
	mockPair := os.Getenv(app.InternalUseDDURLForOPMSEnvVar) == "true" && loopbackURL(rcURL) && loopbackURL(parURL)
	rcKey := r.cfg.GetString("remote_configuration.api_key")
	if key != r.key || rcURL != r.rcURL || parURL != r.parURL || (rcKey != "" && rcKey != key) || r.cfg.GetString("remote_configuration.key") != "" || (rcURL != expectedRC && !mockPair) || (os.Getenv(app.InternalUseDDURLForOPMSEnvVar) == "true" && !mockPair) {
		p.RCReason = "rc_credential_or_endpoint_mismatch"
		return p
	}
	if !rc.Enabled {
		p.RCReason = "rc_org_disabled"
		return p
	}
	if !rc.Authorized {
		p.RCReason = "rc_key_unauthorized"
		return p
	}
	p.RCReady, p.RCReason = true, "rc_ready"
	p.ScopeReady = scope.Granted(now)
	if scope.Reason == "scope_present" && !p.ScopeReady {
		p.ScopeReason = "par_scope_expired"
	}
	return p
}

func enrollmentSite(cfg model.Reader) string {
	site := configutils.ExtractSiteFromURL(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"))
	if site == "" {
		site = "datadoghq.com"
	}
	return site
}

func enrollmentEndpoint(cfg model.Reader) string {
	if os.Getenv(app.InternalUseDDURLForOPMSEnvVar) == "true" {
		return strings.TrimSuffix(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"), "/")
	}
	return "https://api." + enrollmentSite(cfg)
}

func loopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" &&
		(u.Scheme == "http" || u.Scheme == "https") && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
