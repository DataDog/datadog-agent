// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"path/filepath"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/startup"
)

// LoadComponents configures autodiscovery providers and listeners synchronously.
func LoadComponents(ac autodiscovery.Component, config config.Component) {
	LoadComponentsWithTracing(ac, config, nil)
}

// LoadComponentsWithTracing additionally measures setup phases within the current
// Fx startup hook. A nil or disabled recorder leaves setup uninstrumented.
func LoadComponentsWithTracing(ac autodiscovery.Component, config config.Component, recorder *startup.Recorder) {
	confdPath := config.GetString("confd_path")

	confSearchPaths := []string{
		confdPath,
		filepath.Join(defaultpaths.GetDistPath(), "conf.d"),
		"",
	}

	setupAutoDiscovery(confSearchPaths, ac, config, recorder)
}
