// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func initConfig() {
	ddcfg := GlobalConfigBuilder()
	initCommonConfigComponents(ddcfg)
}

func fixupInitConfig() {
	ddcfg := Datadog()
	fixupInitCommonConfigComponents(ddcfg)
	fixupInitServerlessOnlyComponents(ddcfg)
}

// called only for full-agent, ONLY for serverless, after declaring settings
func fixupInitServerlessOnlyComponents(_ pkgconfigmodel.Config) { // nolint:unused,deadcode this is only used by serverless
	// Do not extend this list !
	//
	// Those are legacy behavior that should not be extend. The same config must be compatible with any Agent no
	// matter its version (serverless, full, IOT, ...). We must be able to look at a configuration and deduce
	// exactly the Agent behavior. The only acceptable difference are OS defaults (path, port, ...).
	//
	// If some product are not compatible with serverless, that difference in behavior must be handle within those
	// products not through configuration defaults manipulation.

	// Those setting are disable to silence logs but do not entirely disable the data collection for k8s or sbom.
	pkgconfigmodel.AddOverrideFunc(func(config pkgconfigmodel.Config) {

		for name, defaultVal := range map[string]interface{}{
			"sbom.container_image.exclude_pause_container": false,
			"kubernetes_persistent_volume_claims_as_tags":  false,
			"kubernetes_node_annotations_as_tags":          map[string]string{},
		} {
			if !config.IsConfigured(name) {
				log.Debugf("Disabling %s by default since we are in serverless mode", name)
				config.Set(name, defaultVal, pkgconfigmodel.SourceConfigPostInit)
			}
		}
	})
}
