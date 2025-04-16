// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package setup

import (
	"github.com/DataDog/datadog-agent/pkg/config/create"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// newTestConf generates and returns a new configuration
func newTestConf() pkgconfigmodel.Config {
	conf := create.NewConfig("datadog")
	InitConfig(conf)
	conf.SetTestOnlyDynamicSchema(true)
	conf.SetConfigFile("")
	pkgconfigmodel.ApplyOverrideFuncs(conf)
	return conf
}
