// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LoadComponents configures several common Agent components:
// tagger, collector, scheduler and autodiscovery
func LoadComponents(ctx context.Context, senderManager sender.SenderManager,
	wmeta workloadmeta.Component, confdPath string) {
	sbomScanner, err := scanner.CreateGlobalScanner(config.Datadog)
	if err != nil {
		log.Errorf("failed to create SBOM scanner: %s", err)
	} else if sbomScanner != nil {
		sbomScanner.Start(ctx)
	}

	var t tagger.Tagger

	if config.IsCLCRunner() {
		options, err := remote.CLCRunnerOptions()
		if err != nil {
			log.Errorf("unable to configure the remote tagger: %s", err)
			t = local.NewFakeTagger()
		} else if options.Disabled {
			log.Info("remote tagger is disabled")
			t = local.NewFakeTagger()
		} else {
			t = remote.NewTagger(options)
		}
	} else {
		t = local.NewTagger(wmeta)
	}

	tagger.SetDefaultTagger(t)
	if err := tagger.Init(ctx); err != nil {
		log.Errorf("failed to start the tagger: %s", err)
	}

	// create the Collector instance and start all the components
	// NOTICE: this will also setup the Python environment, if available
	Coll = collector.NewCollector(senderManager, GetPythonPaths()...)

	// setup autodiscovery
	confSearchPaths := []string{
		confdPath,
		filepath.Join(path.GetDistPath(), "conf.d"),
		"",
	}

	// setup autodiscovery. must be done after the tagger is initialized
	// because of subscription to metadata store.
	AC = setupAutoDiscovery(confSearchPaths, scheduler.NewMetaScheduler())
}
