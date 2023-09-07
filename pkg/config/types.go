// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/conf"
)

// Proxy represents the configuration for proxies in the agent
type Proxy = conf.Proxy

// ConfigReader is a subset of Config that only allows reading of configuration
type ConfigReader = conf.ConfigReader

type ConfigWriter = conf.ConfigWriter

type ConfigReaderWriter = conf.ConfigReaderWriter

type ConfigLoader = conf.ConfigLoader

// Config represents an object that can load and store configuration parameters
// coming from different kind of sources:
// - defaults
// - files
// - environment variables
// - flags
type Config = conf.Config

var NewConfig = conf.NewConfig
