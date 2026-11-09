// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"net/url"
	"os"
	"time"

	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
)

type Observation struct {
	Ready  bool
	Reason phonehomedef.Reason
}

// Source observes startup prerequisites and releases discovery demand after handoff.
type Source interface {
	Read(time.Time) Observation
	Release()
}

type rcSource struct {
	cfg                model.Reader
	service            rcservice.Component
	key, rcURL, parURL string
}

// NewSource uses only the existing RC accessor. Positive RC is sufficient in this slice.
func NewSource(cfg model.Reader, service rcservice.Component) Source {
	return &rcSource{cfg: cfg, service: service, key: cfg.GetString("api_key"), rcURL: configutils.GetMainEndpoint(cfg, "https://config.", par.RemoteConfigURL), parURL: enrollmentEndpoint(cfg)}
}

// Release is a no-op: RC polling is shared with the rest of core Agent.
func (*rcSource) Release() {}

func (r *rcSource) Read(_ time.Time) Observation {
	if !configutils.IsRemoteConfigEnabled(r.cfg) {
		return Observation{Reason: phonehomedef.ReasonRCLocallyDisabled}
	}
	if r.service == nil {
		return Observation{Reason: phonehomedef.ReasonRCServiceUnavailable}
	}
	key := r.cfg.GetString("api_key")
	rcURL := configutils.GetMainEndpoint(r.cfg, "https://config.", par.RemoteConfigURL)
	parURL, err := par.APIBaseURL(r.cfg, par.Site(r.cfg))
	if err != nil {
		return Observation{Reason: phonehomedef.ReasonInvalidEndpoint}
	}
	mockPair := os.Getenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS") == "true" && loopbackURL(rcURL) && loopbackURL(parURL)
	rcKey := r.cfg.GetString("remote_configuration.api_key")
	if key == "" || key != r.key || rcURL != r.rcURL || parURL != r.parURL || (rcKey != "" && rcKey != key) || r.cfg.GetString("remote_configuration.key") != "" ||
		(os.Getenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS") == "true" && !mockPair) {
		return Observation{Reason: phonehomedef.ReasonRCCredentialOrEndpointMismatch}
	}
	rc := r.service.GetOrgStatus()
	if rc == nil {
		return Observation{Reason: phonehomedef.ReasonRCUnchecked}
	}
	if !rc.Enabled {
		return Observation{Reason: phonehomedef.ReasonRCOrgDisabled}
	}
	if !rc.Authorized {
		return Observation{Reason: phonehomedef.ReasonRCKeyUnauthorized}
	}
	return Observation{Ready: true, Reason: phonehomedef.ReasonRCReady}
}

func enrollmentEndpoint(cfg model.Reader) string {
	// Read rejects endpoint errors before discovery can dispatch.
	endpoint, err := par.APIBaseURL(cfg, par.Site(cfg))
	if err != nil {
		return ""
	}
	return endpoint
}

func loopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" &&
		(u.Scheme == "http" || u.Scheme == "https") && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
