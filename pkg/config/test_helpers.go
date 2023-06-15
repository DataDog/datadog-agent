// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func SetupConf() Config {
	conf := NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	InitConfig(conf)
	return conf
}

func SetupConfFromYAML(yamlConfig string) Config {
	conf := SetupConf()
	conf.SetConfigType("yaml")
	e := conf.ReadConfig(bytes.NewBuffer([]byte(yamlConfig)))
	if e != nil {
		log.Println(e)
	}
	return conf
}

// ResetSystemProbeConfig resets the configuration.
func ResetSystemProbeConfig(t *testing.T) {
	originalConfig := SystemProbe
	t.Cleanup(func() {
		SystemProbe = originalConfig
	})
	SystemProbe = NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	InitSystemProbeConfig(SystemProbe)
}
