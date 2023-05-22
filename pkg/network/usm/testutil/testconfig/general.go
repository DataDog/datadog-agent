// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package testconfig

import (
	"os"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func USMTestConfig() *config.Config {
	_, _ = sysconfig.New("")
	cfg := config.New()
	if os.Getenv("BPF_DEBUG") != "" {
		cfg.BPFDebug = true
	}
	return cfg
}
