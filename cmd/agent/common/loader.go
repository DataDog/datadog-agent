// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// LoadComponents configures several common Agent components:
// tagger, collector, scheduler and autodiscovery
func LoadComponents(_ secrets.Component, wmeta workloadmeta.Component, taggerComp tagger.Component, filterStore workloadfilter.Component, ac autodiscovery.Component, confdPath string) {
	confSearchPaths := []string{
		confdPath,
		filepath.Join(defaultpaths.GetDistPath(), "conf.d"),
		"",
	}

	setupAutoDiscovery(confSearchPaths, wmeta, taggerComp, filterStore, ac)
}
