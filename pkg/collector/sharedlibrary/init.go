// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharedlibrary implements the layer to interact shared library-based checks.
package sharedlibrary

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// init adds the shared library loader to the scheduler
func init() {
	if pkgconfigsetup.Datadog().GetBool("shared_libraries_check.enabled") {
		factory := func(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filter workloadfilter.Component) (check.Loader, int, error) {
			loader, err := NewSharedLibraryCheckLoader(senderManager, logReceiver, tagger, filter, defaultSharedLibraryLoader)
			priority := 40
			return loader, priority, err
		}

		loaders.RegisterLoader(factory)
	} else {
		log.Info("Shared libraries checks are disabled.")
	}
}
