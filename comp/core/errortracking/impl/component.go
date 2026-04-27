// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package errortrackingimpl

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	errortracking "github.com/DataDog/datadog-agent/comp/core/errortracking/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const httpClientResetInterval = 5 * time.Minute

// Requires lists the Fx dependencies of the component.
type Requires struct {
	Lc     compdef.Lifecycle
	Log    log.Component
	Config config.Component
}

// Provides exposes the component to consumers (Worker 3 wires this into the
// pkg/util/log/errortracking Pipeline as its Sender).
type Provides struct {
	Comp errortracking.Component
}

// NewComponent constructs the COAT error tracking sender from agent config.
// The component is always provided; if errortracking is disabled in config,
// Worker 3's setup code skips installing the handler so Send is never called.
func NewComponent(reqs Requires) (Provides, error) {
	cfg := reqs.Config

	url := pkgconfigutils.GetMainEndpoint(cfg, intakeHostPrefix, "errortracking.dd_url")
	url = strings.TrimRight(url, "/") + intakePath
	apiKey := pkgconfigutils.SanitizeAPIKey(cfg.GetString("api_key"))

	agentVersion, _ := version.Agent()

	client := httputils.NewResetClient(
		httpClientResetInterval,
		httpClientFactory(cfg, httpClientTimeout),
	)

	sender := newSenderImpl(
		reqs.Log,
		client,
		url,
		apiKey,
		agentVersion.GetNumberAndPre(),
		cfg.GetString("hostname"),
	)

	reqs.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error { return nil },
		OnStop: func(_ context.Context) error {
			sender.markStopped()
			return nil
		},
	})

	return Provides{Comp: sender}, nil
}

func httpClientFactory(cfg config.Component, timeout time.Duration) func() *http.Client {
	return func() *http.Client {
		return &http.Client{
			Timeout:   timeout,
			Transport: httputils.CreateHTTPTransport(cfg),
		}
	}
}
