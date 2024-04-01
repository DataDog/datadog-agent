// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostimpl implements a component to generate the 'host' metadata payload (also known as "v5").
package hostimpl

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	hostComp "github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// run the host metadata collector every 1800 seconds (30 minutes)
const defaultCollectInterval = 1800 * time.Second

// the host metadata collector interval can be set through configuration within acceptable bounds
const minAcceptedInterval = 300   // 5min
const maxAcceptedInterval = 14400 // 4h

const providerName = "host"

type host struct {
	log       log.Component
	config    config.Component
	resources resources.Component

	hostname        string
	collectInterval time.Duration
	serializer      serializer.MetricSerializer
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
}

type provides struct {
	fx.Out

	Comp                 hostComp.Component
	MetadataProvider     runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
}

func newHostProvider(deps dependencies) provides {
	collectInterval := defaultCollectInterval
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
				break
			}
		}
	}

	hname, _ := hostname.Get(context.Background())
	h := host{
		log:             deps.Log,
		config:          deps.Config,
		resources:       deps.Resources,
		hostname:        hname,
		collectInterval: collectInterval,
		serializer:      deps.Serializer,
	}
	return provides{
		Comp:             &h,
		MetadataProvider: runnerimpl.NewProvider(h.collect),
		FlareProvider:    flaretypes.NewProvider(h.fillFlare),
		StatusHeaderProvider: status.NewHeaderInformationProvider(StatusProvider{
			Config: h.config,
		}),
	}
}

func (h *host) collect(ctx context.Context) time.Duration {
	payload := h.getPayload(ctx)
	if err := h.serializer.SendHostMetadata(payload); err != nil {
		h.log.Errorf("unable to submit host metadata payload, %s", err)
	}
	return h.collectInterval
}

func (h *host) GetPayloadAsJSON(ctx context.Context) ([]byte, error) {
	return json.MarshalIndent(h.getPayload(ctx), "", "    ")
}

func (h *host) fillFlare(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc(filepath.Join("metadata", "host.json"), func() ([]byte, error) { return h.GetPayloadAsJSON(context.Background()) })
}
