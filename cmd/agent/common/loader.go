// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector"
	lsched "github.com/DataDog/datadog-agent/pkg/logs/scheduler"
	lstatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// LoadComponents configures several common Agent components:
// tagger, collector, scheduler and autodiscovery
func LoadComponents(confdPath string) {
	// TODO(juliogreff): pass a local store to tagger and AD maybe? Also,
	// other agents may need to initialize this as well.
	workloadmeta.GetGlobalStore().Run(context.Background())

	// start the tagger. must be done before autodiscovery, as it needs to
	// be the first subscribed to metadata store to avoid race conditions.
	tagger.SetDefaultTagger(local.NewTagger(collectors.DefaultCatalog))
	tagger.Init()

	// create the Collector instance and start all the components
	// NOTICE: this will also setup the Python environment, if available
	Coll = collector.NewCollector(GetPythonPaths()...)

	// creating the meta scheduler
	metaScheduler := scheduler.NewMetaScheduler()

	// registering the check scheduler
	metaScheduler.Register("check", collector.InitCheckScheduler(Coll))

	// registering the logs scheduler
	if lstatus.Get().IsRunning {
		metaScheduler.Register("logs", lsched.GetScheduler())
	}

	// setup autodiscovery
	confSearchPaths := []string{
		confdPath,
		filepath.Join(GetDistPath(), "conf.d"),
		"",
	}

	// setup autodiscovery. must be done after the tagger is initialized
	// because of subscription to metadata store.
	AC = setupAutoDiscovery(confSearchPaths, metaScheduler)
}
