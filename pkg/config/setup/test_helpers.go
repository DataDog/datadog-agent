// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"bytes"
	"log"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// conf generates and returns a new configuration
func conf() pkgconfigmodel.Config {
	conf := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	InitConfig(conf)
	return conf
}

// confFromYAML generates a configuration from the given yaml config
func confFromYAML(yamlConfig string) pkgconfigmodel.Config {
	conf := conf()
	conf.SetConfigType("yaml")
	e := conf.ReadConfig(bytes.NewBuffer([]byte(yamlConfig)))
	if e != nil {
		log.Println(e)
	}
	return conf
}
