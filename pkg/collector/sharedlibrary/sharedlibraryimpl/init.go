// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

// Package sharedlibrarycheck implements the layer to interact shared library-based checks
package sharedlibrarycheck

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// InitSharedLibraryChecksLoader adds the shared library checks loader to the scheduler
func InitSharedLibraryChecksLoader() {
	libFolderPath := pkgconfigsetup.Datadog().GetString("shared_library_check.library_folder_path")

	factory := func(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filter workloadfilter.Component) (check.Loader, int, error) {
		priority := 40

		sharedLibraryLoader, err := ffi.NewSharedLibraryLoader(libFolderPath)
		if err != nil {
			return nil, priority, err
		}

		loader, err := newCheckLoader(senderManager, logReceiver, tagger, filter, sharedLibraryLoader)

		return loader, priority, err
	}

	loaders.RegisterLoader(factory)

	log.Infof("Shared library checks are enabled. Looking for shared libraries in %q.", libFolderPath)
	log.Warn("Shared library checks are still experimental. Be careful when using it, breaking changes may be made.")
}
