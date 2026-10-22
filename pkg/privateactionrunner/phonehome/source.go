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
)

type Observation struct {
	Ready  bool
	Reason string
}

// Source observes startup prerequisites without performing network requests.
type Source interface{ Read(time.Time) Observation }

type rcSource struct {
	cfg                model.Reader
	service            rcservice.Component
	key, rcURL, parURL string
}

// NewSource uses only the existing RC accessor. Positive RC is sufficient in this slice.
func NewSource(cfg model.Reader, service rcservice.Component) Source {
	return &rcSource{cfg: cfg, service: service, key: cfg.GetString("api_key"), rcURL: configutils.GetMainEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url"), parURL: enrollmentEndpoint(cfg)}
}

func (r *rcSource) Read(_ time.Time) Observation {
	if !configutils.IsRemoteConfigEnabled(r.cfg) {
		return Observation{Reason: "rc_locally_disabled"}
	}
	if r.service == nil {
		return Observation{Reason: "rc_service_unavailable"}
	}
	key := r.cfg.GetString("api_key")
	rcURL := configutils.GetMainEndpoint(r.cfg, "https://config.", "remote_configuration.rc_dd_url")
	parURL := enrollmentEndpoint(r.cfg)
	mockPair := os.Getenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS") == "true" && loopbackURL(rcURL) && loopbackURL(parURL)
	rcKey := r.cfg.GetString("remote_configuration.api_key")
	if key == "" || key != r.key || rcURL != r.rcURL || parURL != r.parURL || (rcKey != "" && rcKey != key) || r.cfg.GetString("remote_configuration.key") != "" ||
		(strings.TrimSuffix(rcURL, ".") != "https://config."+enrollmentSite(r.cfg) && !mockPair) || (os.Getenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS") == "true" && !mockPair) {
		return Observation{Reason: "rc_credential_or_endpoint_mismatch"}
	}
	rc := r.service.GetOrgStatus()
	if rc == nil {
		return Observation{Reason: "rc_unchecked"}
	}
	if !rc.Enabled {
		return Observation{Reason: "rc_org_disabled"}
	}
	if !rc.Authorized {
		return Observation{Reason: "rc_key_unauthorized"}
	}
	return Observation{Ready: true, Reason: "rc_ready"}
}

func enrollmentSite(cfg model.Reader) string {
	site := configutils.ExtractSiteFromURL(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"))
	if site == "" {
		site = "datadoghq.com"
	}
	return site
}

func enrollmentEndpoint(cfg model.Reader) string {
	if os.Getenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS") == "true" {
		return strings.TrimSuffix(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"), "/")
	}
	return "https://api." + enrollmentSite(cfg)
}

func loopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" &&
		(u.Scheme == "http" || u.Scheme == "https") && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
