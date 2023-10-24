// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package resources

import (
	"context"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/gohai/processes"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

const defaultCollectInterval = 300 * time.Second
const providerName = "resources"

type resources struct {
	log log.Component

	hostname        string
	collectInterval time.Duration
	serializer      serializer.MetricSerializer
}

type dependencies struct {
	fx.In

	// Resources is enabled by default for most binaries. But even for binaries where we don't want to send the
	// 'resources' paylod, like dogstatsd, we still need the resources Component. This is because the resources data
	// is embedded in other metadata payload like 'host'. This means that even if resources is disabled it might be
	// required in the build in order for the `Get` method to be available.
	//
	// This is why we have a Params struct for resources. It's `optional` so most of the binaries don't have to
	// supply a Params struct but only need to import the metadata.Bundle.
	Params *Params `optional:"true"`

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
}

type provides struct {
	fx.Out

	Comp     Component
	Provider runner.Provider
}

func newResourcesProvider(deps dependencies) provides {
	// By default, the 'resources' metadata is only enabled on Linux. Users can manually enable it on darwin
	// platform. This is legacy behavior from Agent V5.
	enabled := runtime.GOOS == "linux"

	collectInterval := defaultCollectInterval
	confProviders, err := configUtils.GetMetadataProviders(deps.Config)
	if err != nil {
		deps.Log.Errorf("Error parsing metadata provider configuration, falling back to default behavior: %s", err)
	} else {
		for _, p := range confProviders {
			if p.Name == providerName {
				// user configured interval take precedence over the default one
				collectInterval = p.Interval * time.Second
				enabled = true
				break
			}
		}
	}

	hname, _ := hostname.Get(context.Background())
	r := resources{
		log:             deps.Log,
		hostname:        hname,
		collectInterval: collectInterval,
		serializer:      deps.Serializer,
	}
	res := provides{
		Comp:     &r,
		Provider: runner.NewEmptyProvider(),
	}

	if deps.Params != nil && deps.Params.Disabled {
		deps.Log.Infof("resources metadata provider is not available for this binary")
	} else if !enabled {
		deps.Log.Infof("resources metadata provider is disabled from the configuration")
	} else if collectInterval == 0 {
		deps.Log.Infof("Collection interval for 'resources' metadata provider is set to 0: disabling it")
	} else {
		deps.Log.Debugf("Collection interval for 'resources' metadata provider is set to %s", collectInterval)
		res.Provider = runner.NewProvider(r.collect)
	}

	return res
}

// For testing purposes
var collectResources = gohaiResourcesCollect

func gohaiResourcesCollect() (interface{}, error) {
	info, err := processes.CollectInfo()
	if err != nil {
		return nil, err
	}
	processes, _, err := info.AsJSON()
	return processes, err
}

// Get returns the resources payload to be used in other metadata providers.
func (r *resources) Get() map[string]interface{} {
	proc, err := collectResources()
	if err != nil {
		r.log.Warnf("Failed to retrieve processes metadata from gohai: %s", err)
		return nil
	}

	// The format dates back from Agent V5
	return map[string]interface{}{
		"resources": map[string]interface{}{
			"processes": map[string]interface{}{
				"snaps": []interface{}{proc},
			},
			"meta": map[string]string{
				"host": r.hostname,
			},
		},
	}
}

func (r *resources) collect(_ context.Context) time.Duration {
	payload := r.Get()

	if payload != nil {
		if err := r.serializer.SendProcessesMetadata(payload); err != nil {
			r.log.Errorf("unable to serialize processes metadata payload, %s", err)
		}
	}
	return r.collectInterval
}
