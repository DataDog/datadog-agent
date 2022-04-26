// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package modules

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	rc "github.com/DataDog/datadog-agent/pkg/runtimecompiler"
	compilerconfig "github.com/DataDog/datadog-agent/pkg/runtimecompiler/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	statsdPoolSize = 64
)

// Compiler Factory
var Compiler = module.Factory{
	Name:             config.CompilerModule,
	ConfigNamespaces: []string{},
	Fn: func(cfg *config.Config) (module.Module, error) {
		log.Infof("Starting the runtime compilation module")

		rcConfig := compilerconfig.NewConfig(cfg)

		statsdClient, err := getStatsdClient(rcConfig)
		if err != nil {
			log.Errorf("error creating statsd client for runtime compiler: %s", err)
			statsdClient = nil
		}

		rc.RuntimeCompiler.Init(rcConfig, statsdClient)
		err = rc.RuntimeCompiler.Run()
		if err != nil {
			return nil, fmt.Errorf("Runtime compilation error: %w", err)
		}

		log.Infof("Runtime compilation module completed successfully")
		return &compilerModule{}, nil
	},
}

var _ module.Module = &compilerModule{}

type compilerModule struct{}

func (c *compilerModule) GetStats() map[string]interface{} {
	return nil
}

func (c *compilerModule) Register(_ *module.Router) error {
	return nil
}

func (c *compilerModule) Close() {}

func getStatsdClient(cfg *compilerconfig.Config) (statsd.ClientInterface, error) {
	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		statsdAddr = cfg.StatsdAddr
	}

	return statsd.New(statsdAddr, statsd.WithBufferPoolSize(statsdPoolSize))
}
