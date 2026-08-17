// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// knownOnlySetup wraps a Setup and forwards only SetKnown calls, ignoring
// defaults, env bindings, and all other schema-building operations.
// It is used to register system-probe config keys as known in the core agent
// config so that checkKnownKey does not emit false-positive warnings when the
// core agent reads a datadog.yaml that contains system-probe sections.
type knownOnlySetup struct {
	inner pkgconfigmodel.Setup
}

func (k *knownOnlySetup) SetDefault(key string, _ interface{}) {
	k.inner.SetKnown(strings.ToLower(key))
}

func (k *knownOnlySetup) BindEnvAndSetDefault(key string, _ interface{}, _ ...string) {
	k.inner.SetKnown(strings.ToLower(key))
}

func (k *knownOnlySetup) SetKnown(key string)                                                      { k.inner.SetKnown(key) }
func (k *knownOnlySetup) BuildSchema()                                                              {}
func (k *knownOnlySetup) SetEnvPrefix(_ string)                                                    {}
func (k *knownOnlySetup) SetEnvKeyReplacer(_ *strings.Replacer)                                    {}
func (k *knownOnlySetup) ParseEnvSplitComma(_ string)                                              {}
func (k *knownOnlySetup) ParseEnvSplitSpace(_ string)                                              {}
func (k *knownOnlySetup) ParseEnvJSON(_ string, _ any)                                             {}
func (k *knownOnlySetup) ParseEnvAsStringSlice(_ string, _ func(string) []string)                 {}
func (k *knownOnlySetup) ParseEnvAsMapStringInterface(_ string, _ func(string) map[string]interface{}) {
}
func (k *knownOnlySetup) AddConfigPath(_ string)               {}
func (k *knownOnlySetup) AddExtraConfigPaths(_ []string) error { return nil }
func (k *knownOnlySetup) SetConfigName(_ string)               {}
func (k *knownOnlySetup) SetConfigFile(_ string)               {}
func (k *knownOnlySetup) SetConfigType(_ string)               {}
