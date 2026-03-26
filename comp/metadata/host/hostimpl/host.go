// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostimpl implements a component to generate the 'host' metadata payload (also known as "v5").
package hostimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/cenkalti/backoff/v5"
	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	hostComp "github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/gohai"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// run the host metadata collector every 1800 seconds (30 minutes)
const defaultCollectInterval = 1800 * time.Second

// start the host metadata collector with an early interval of 300 seconds (5 minutes)
const defaultEarlyInterval = 300 * time.Second

// the host metadata collector interval can be set through configuration within acceptable bounds
const minAcceptedInterval = 60    // 1min
const maxAcceptedInterval = 14400 // 4h

const providerName = "host"

type host struct {
	log          log.Component
	config       config.Component
	resources    resources.Component
	hostnameComp hostnameinterface.Component

	hostname      string
	serializer    serializer.MetricSerializer
	backoffPolicy *backoff.ExponentialBackOff
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newHostProvider),
	)
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Resources  resources.Component
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

type provides struct {
	fx.Out

	Comp                 hostComp.Component
	MetadataProvider     runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
	Endpoint             api.AgentEndpointProvider
	GohaiEndpoint        api.AgentEndpointProvider
}

func newHostProvider(deps dependencies) provides {
	collectInterval := defaultCollectInterval
	earlyInterval := defaultEarlyInterval
	confProviders, err := configUtils.GetMetadataProviders(deps.Config)
	if err != nil {
		deps.Log.Errorf("Error parsing metadata provider configuration, falling back to default behavior: %s", err)
	} else {
		for _, p := range confProviders {
			if p.Name == providerName {
				if p.Interval < minAcceptedInterval || p.Interval > maxAcceptedInterval {
					deps.Log.Errorf("Ignoring host metadata interval: %v is outside of accepted values (min: %v, max: %v)", p.Interval, minAcceptedInterval, maxAcceptedInterval)
					break
				}
				// user configured interval take precedence over the default one
				collectInterval = p.Interval * time.Second

				if p.EarlyInterval > 0 {
					if p.EarlyInterval < minAcceptedInterval || p.EarlyInterval > maxAcceptedInterval {
						deps.Log.Errorf("Ignoring host metadata early interval: %v is outside of accepted values (min: %v, max: %v)", p.EarlyInterval, minAcceptedInterval, maxAcceptedInterval)
						break
					}
					if p.EarlyInterval > p.Interval {
						deps.Log.Errorf("Ignoring host metadata early interval: %v is greater than main interval %v", p.EarlyInterval, p.Interval)
						break
					}
					// user configured early interval take precedence over the default one
					earlyInterval = p.EarlyInterval * time.Second
				}

				break
			}
		}
	}

	hname, _ := deps.Hostname.Get(context.Background())

	// exponential backoff for collection intervals which arrives at user's configured interval
	bo := &backoff.ExponentialBackOff{
		InitialInterval:     earlyInterval, // start with the early interval
		RandomizationFactor: 0,
		Multiplier:          3.0,
		MaxInterval:         collectInterval, // max interval is the user configured interval
	}
	bo.Reset()

	h := host{
		log:           deps.Log,
		config:        deps.Config,
		resources:     deps.Resources,
		hostnameComp:  deps.Hostname,
		hostname:      hname,
		serializer:    deps.Serializer,
		backoffPolicy: bo,
	}
	return provides{
		Comp:             &h,
		MetadataProvider: runnerimpl.NewProvider(h.collect),
		FlareProvider:    flaretypes.NewProvider(h.fillFlare),
		StatusHeaderProvider: status.NewHeaderInformationProvider(StatusProvider{
			Config:   h.config,
			Hostname: h.hostnameComp,
		}),
		Endpoint:      api.NewAgentEndpointProvider(h.writePayloadAsJSON, "/metadata/v5", "GET"),
		GohaiEndpoint: api.NewAgentEndpointProvider(h.writeGohaiPayload, "/metadata/gohai", "GET"),
	}
}

func (h *host) collect(ctx context.Context) time.Duration {
	payload := h.getPayload(ctx)

	nextInterval := h.backoffPolicy.NextBackOff()
	if nextInterval <= 0 || nextInterval > h.backoffPolicy.MaxInterval {
		nextInterval = h.backoffPolicy.MaxInterval
	}

	// Debug log to show the actual interval that will be used
	h.log.Debugf("Next host metadata collection scheduled in %s", nextInterval)

	if err := h.serializer.SendHostMetadata(payload); err != nil {
		h.log.Errorf("unable to submit host metadata payload, %s", err)
	}

	return nextInterval
}

func (h *host) GetPayloadAsJSON(ctx context.Context) ([]byte, error) {
	return json.MarshalIndent(h.getPayload(ctx), "", "    ")
}

func (h *host) fillFlare(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc(filepath.Join("metadata", "host.json"), func() ([]byte, error) { return h.GetPayloadAsJSON(context.Background()) })
}

func (h *host) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	jsonPayload, err := h.GetPayloadAsJSON(context.Background())
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to marshal v5 metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to scrub metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}

func (h *host) writeGohaiPayload(w http.ResponseWriter, _ *http.Request) {
	payload := gohai.GetPayloadWithProcesses(h.hostname, h.config.GetBool("metadata_ip_resolution_from_hostname"), env.IsContainerized())
	jsonPayload, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to marshal gohai metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		httputils.SetJSONError(w, h.log.Errorf("Unable to scrub gohai metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}
