// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

// Package sharedlibrarycheck implements the layer to interact shared library-based checks
package sharedlibrarycheck

import (
	"context"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/enrichment"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// InitSharedLibraryChecksLoader adds the shared library checks loader to the scheduler
func InitSharedLibraryChecksLoader() {
	libFolderPath := pkgconfigsetup.Datadog().GetString("shared_library_check.library_folder_path")

	// Build enrichment data from agent subsystems
	hostname, err := hostnameUtil.Get(context.TODO())
	if err != nil {
		log.Warnf("Failed to get hostname for enrichment data: %s", err)
		hostname = ""
	}

	enrichmentData := enrichment.EnrichmentData{
		Hostname:         hostname,
		AgentVersion:     version.AgentVersion,
		HostTags:         map[string]string{},
		ConfigValues:     map[string]interface{}{},
		ProcessStartTime: uint64(pkgconfigsetup.StartTime.Unix()),
	}

	enrichmentProvider, err := enrichment.NewStaticProvider(enrichmentData)
	if err != nil {
		log.Errorf("Failed to create enrichment provider: %s", err)
		return
	}

	factory := func(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filter workloadfilter.Component) (check.Loader, int, error) {
		sharedLibraryLoader := ffi.NewSharedLibraryLoader(libFolderPath)
		loader, err := newCheckLoader(senderManager, logReceiver, tagger, filter, sharedLibraryLoader, enrichmentProvider)
		priority := 40
		return loader, priority, err
	}

	loaders.RegisterLoader(factory)

	log.Infof("Shared library checks are enabled. Looking for shared libraries in %q.", libFolderPath)
	log.Warn("Shared library checks are still experimental. Be careful when using it, breaking changes may be made.")
}
