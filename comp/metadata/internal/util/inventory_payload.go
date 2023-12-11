// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package util offers helpers and building blocks to easily generate payloads for the inventory product.
//
// Usage: When creating a new payload for the inventory product, one should only embed the 'InventoryPayload' struct and
// provide it with a callback to generate a payload.
//
// Example:
//
//	type customPayload struct {
//		InventoryPayload
//	}
//
//	type dependencies struct {
//		fx.In
//
//		Log        log.Component
//		Config     config.Component
//		Serializer serializer.MetricSerializer
//	}
//
//	type provides struct {
//		fx.Out
//
//		Comp          Component
//		Provider      runner.Provider
//		FlareProvider flaretypes.Provider
//	}
//
//	func newCustomPayload(deps dependencies) provides {
//		cp := &customPayload{
//			InventoryPayload: CreateInventoryPayload(
//				deps.Config,
//				deps.Log,
//				deps.Serializer,
//				cp.getPayload,
//				"custom.json",
//			),
//		}
//
//		return provides{
//			Comp:          cp,
//			Provider:      cp.MetadataProvider(),
//			FlareProvider: cp.FlareProvider(),
//		}
//	}
//
// func (cp *customPayload) getPayload() marshaler.JSONMarshaler { ... }
package util

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var (
	defaultMinInterval = 1 * time.Minute
	defaultMaxInterval = 10 * time.Minute

	// For testing purposes
	timeSince = time.Since
)

// PayloadGetter is the callback to generate a new payload exposed by each inventory payload to InventoryPayload utils
type PayloadGetter func() marshaler.JSONMarshaler

// InventoryPayload offers helpers for all inventory payloads providing all the common part to create a new payload.
// InventoryPayload will handle the common configuration as well as refresh rates and flare. This type is meant to be
// embedded.
//
// Embedding type need to provide a PayloadGetter callback when calling Init. This callback will be called each time a
// new payload need to be generated.
type InventoryPayload struct {
	m sync.Mutex

	conf       config.Component
	log        log.Component
	serializer serializer.MetricSerializer
	getPayload PayloadGetter

	Enabled       bool
	LastCollect   time.Time
	MinInterval   time.Duration
	MaxInterval   time.Duration
	ForceRefresh  bool
	flareFileName string
}

// CreateInventoryPayload returns an initialized InventoryPayload. 'getPayload' will be called each time a new payload
// needs to be generated.
func CreateInventoryPayload(conf config.Component, l log.Component, s serializer.MetricSerializer, getPayload PayloadGetter, flareFileName string) InventoryPayload {
	minInterval := time.Duration(conf.GetInt("inventories_min_interval")) * time.Second
	if minInterval <= 0 {
		minInterval = defaultMinInterval
	}

	maxInterval := time.Duration(conf.GetInt("inventories_max_interval")) * time.Second
	if maxInterval <= 0 {
		maxInterval = defaultMaxInterval
	}

	return InventoryPayload{
		Enabled:       InventoryEnabled(conf),
		conf:          conf,
		log:           l,
		serializer:    s,
		getPayload:    getPayload,
		flareFileName: flareFileName,
		MinInterval:   minInterval,
		MaxInterval:   maxInterval,
	}
}

// FlareProvider returns a flare providers to add the current inventory payload to each flares.
func (i *InventoryPayload) FlareProvider() flaretypes.Provider {
	return flaretypes.NewProvider(i.fillFlare)
}

// MetadataProvider returns a metadata 'runner.Provider' for the current inventory payload (taking into account if
// invnetory is enabled or not).
func (i *InventoryPayload) MetadataProvider() runnerimpl.Provider {
	if i.Enabled {
		return runnerimpl.NewProvider(i.collect)
	}
	return runnerimpl.NewEmptyProvider()
}

// collect is the callback expected by the metadata runner.Provider. It will send a new payload and return the next
// interval to be called.
func (i *InventoryPayload) collect(_ context.Context) time.Duration {
	i.m.Lock()
	defer i.m.Unlock()

	// Collect will be called every MinInterval second. We send a new payload if a refresh was trigger or if it's
	// been at least MaxInterval seconds since the last payload.
	if !i.ForceRefresh && i.MaxInterval-timeSince(i.LastCollect) > 0 {
		return i.MinInterval
	}

	i.ForceRefresh = false
	i.LastCollect = time.Now()

	p := i.getPayload()
	if err := i.serializer.SendMetadata(p); err != nil {
		i.log.Errorf("unable to submit inventories payload, %s", err)
	}
	return i.MinInterval
}

// Refresh trigger a new payload to be send while still respecting the minimal interval between two updates.
func (i *InventoryPayload) Refresh() {
	if !i.Enabled {
		return
	}

	i.m.Lock()
	defer i.m.Unlock()

	// For a refresh we want to resend a new payload as soon as possible but still respect MinInterval second
	// since the last update. The Refresh method set ForceRefresh to true which will trigger a new payload when
	// Collect is called every MinInterval.
	i.ForceRefresh = true
}

// GetAsJSON returns the payload as a JSON string. Useful to be displayed in the CLI or added to a flare.
func (i *InventoryPayload) GetAsJSON() ([]byte, error) {
	if !i.Enabled {
		return nil, fmt.Errorf("inventory metadata is disabled")
	}

	i.m.Lock()
	defer i.m.Unlock()

	return json.MarshalIndent(i.getPayload(), "", "    ")
}

// fillFlare add the inventory payload to flares.
func (i *InventoryPayload) fillFlare(fb flaretypes.FlareBuilder) error {
	path := filepath.Join("metadata", "inventory", i.flareFileName)
	if !i.Enabled {
		fb.AddFile(path, []byte("inventory metadata is disabled"))
		return nil
	}

	fb.AddFileFromFunc(path, i.GetAsJSON)
	return nil
}
