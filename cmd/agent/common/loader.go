// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/local"
	"github.com/DataDog/datadog-agent/comp/core/tagger/remote"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetWorkloadmetaInit provides the InitHelper for workloadmeta so it can be injected as a Param
// at workloadmeta comp fx injection.
func GetWorkloadmetaInit() workloadmeta.InitHelper {
	return workloadmeta.InitHelper(func(ctx context.Context, wm workloadmeta.Component) error {
		var t tagger.Tagger
		var e error
		if config.IsCLCRunner() {
			options, err := remote.CLCRunnerOptions()
			if err != nil {
				e = fmt.Errorf("unable to configure the remote tagger: %s", err)
				t = local.NewFakeTagger()
			} else if options.Disabled {
				// TODO(components): log the remote tagger being disabled.
				// wm.log.Info("remote tagger is disabled")
				t = local.NewFakeTagger()
			} else {
				t = remote.NewTagger(options)
			}
		} else {
			t = local.NewTagger(wm)
		}

		// SBOM scanner needs to be called here as initialization is required prior to the
		// catalog getting instantiated and initialized.
		sbomScanner, err := scanner.CreateGlobalScanner(config.Datadog)
		if err != nil {
			log.Errorf("failed to create SBOM scanner: %s", err)
		} else if sbomScanner != nil {
			sbomScanner.Start(ctx)
		}

		tagger.SetDefaultTagger(t)
		if err := tagger.Init(ctx); err != nil {
			e = fmt.Errorf("failed to start the tagger: %s", err)
		}

		return e
	})
}

var collectorOnce sync.Once

// LoadCollector instantiate the collector and init the global state 'Coll'.
//
// LoadCollector will initialize the collector only once even if called multiple time. Some command still rely on
// LoadComponents while other setup the collector on their own.
func LoadCollector(senderManager sender.SenderManager) collector.Collector {
	collectorOnce.Do(func() {
		// create the Collector instance and start all the components
		// NOTICE: this will also setup the Python environment, if available
		Coll = collector.NewCollector(senderManager, config.Datadog.GetDuration("check_cancel_timeout"), GetPythonPaths()...)
	})
	return Coll
}

// LoadComponents configures several common Agent components:
// tagger, collector, scheduler and autodiscovery
func LoadComponents(senderManager sender.SenderManager, secretResolver secrets.Component, confdPath string) {
	confSearchPaths := []string{
		confdPath,
		filepath.Join(path.GetDistPath(), "conf.d"),
		"",
	}

	// setup autodiscovery. must be done after the tagger is initialized.

	// TODO(components): revise this pattern.
	// Currently the workloadmeta init hook will initialize the tagger.
	// No big concern here, but be sure to understand there is an implicit
	// assumption about the initializtion of the tagger prior to being here.
	// because of subscription to metadata store.
	AC = setupAutoDiscovery(confSearchPaths, scheduler.NewMetaScheduler(), secretResolver)

	LoadCollector(senderManager)
}
