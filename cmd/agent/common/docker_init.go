// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package common

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func init() {
	// setup docker (for now we enable everything, we might add more option if needed)
	docker.InitDockerUtil(&docker.Config{
		CacheDuration:  10 * time.Second,
		CollectNetwork: true,
		Whitelist:      config.Datadog.GetStringSlice("ac_include"),
		Blacklist:      config.Datadog.GetStringSlice("ac_exclude"),
	})
}
