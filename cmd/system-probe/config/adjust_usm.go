// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxHTTPFrag = 160
)

func adjustUSM(cfg config.Config) {
	deprecateBool(cfg, netNS("enable_http_monitoring"), smNS("enable_http_monitoring"))

	if cfg.GetBool(dsmNS("enabled")) {
		// DSM infers USM
		cfg.Set(smNS("enabled"), true)
	}

	if cfg.GetBool(smNS("enabled")) {
		// USM infers HTTP
		cfg.Set(smNS("enable_http_monitoring"), true)
		applyDefault(cfg, netNS("enable_https_monitoring"), true)
		applyDefault(cfg, spNS("enable_runtime_compiler"), true)
		applyDefault(cfg, spNS("enable_kernel_header_download"), true)
	}

	if cfg.GetBool(smNS("process_service_inference", "enabled")) &&
		!cfg.GetBool(smNS("enabled")) &&
		!cfg.GetBool(dsmNS("enabled")) {
		log.Info("universal service monitoring and data streams monitoring are disabled, disabling process service inference")
		cfg.Set(smNS("process_service_inference", "enabled"), false)
	}

	validateInt(cfg, netNS("http_notification_threshold"), cfg.GetInt(spNS("max_tracked_connections"))/2, func(v int) error {
		limit := cfg.GetInt(netNS("max_tracked_http_connections"))
		if v >= limit {
			return fmt.Errorf("notification threshold %d set higher than tracked connections %d", v, limit)
		}
		return nil
	})

	limitMaxInt64(cfg, netNS("http_max_request_fragment"), maxHTTPFrag)
}
