// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/fx"

	secconfig "github.com/DataDog/datadog-agent/cmd/security-agent/config"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/internal"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/config,
	// and uses globals in that package.

	// warnings are the warnings generated during setup
	warnings *config.Warnings
}

type dependencies struct {
	fx.In

	Params internal.BundleParams
}

func newConfig(deps dependencies) (Component, error) {
	warnings, err := setupConfig(
		deps.Params.ConfFilePath,
		deps.Params.ConfigName,
		!deps.Params.ConfigLoadSecrets,
		!deps.Params.ConfigMissingOK,
		deps.Params.DefaultConfPath)
	if err != nil {
		return nil, err
	}

	if deps.Params.ConfigLoadSysProbe {
		_, err := sysconfig.Merge(deps.Params.SysProbeConfFilePath)
		if err != nil {
			return &cfg{warnings}, err
		}
	}

	if deps.Params.ConfigLoadSecurityAgent {
		if err := secconfig.Merge(deps.Params.SecurityAgentConfigFilePaths); err != nil {
			return &cfg{warnings}, err
		}
	}

	return &cfg{warnings}, nil
}

func (c *cfg) IsSet(key string) bool {
	return config.Datadog.IsSet(key)
}
func (c *cfg) Get(key string) interface{} {
	return config.Datadog.Get(key)
}
func (c *cfg) GetString(key string) string {
	return config.Datadog.GetString(key)
}
func (c *cfg) GetBool(key string) bool {
	return config.Datadog.GetBool(key)
}
func (c *cfg) GetInt(key string) int {
	return config.Datadog.GetInt(key)
}
func (c *cfg) GetInt32(key string) int32 {
	return config.Datadog.GetInt32(key)
}
func (c *cfg) GetInt64(key string) int64 {
	return config.Datadog.GetInt64(key)
}
func (c *cfg) GetFloat64(key string) float64 {
	return config.Datadog.GetFloat64(key)
}
func (c *cfg) GetTime(key string) time.Time {
	return config.Datadog.GetTime(key)
}
func (c *cfg) GetDuration(key string) time.Duration {
	return config.Datadog.GetDuration(key)
}
func (c *cfg) GetStringSlice(key string) []string {
	return config.Datadog.GetStringSlice(key)
}
func (c *cfg) GetFloat64SliceE(key string) ([]float64, error) {
	return config.Datadog.GetFloat64SliceE(key)
}
func (c *cfg) GetStringMap(key string) map[string]interface{} {
	return config.Datadog.GetStringMap(key)
}
func (c *cfg) GetStringMapString(key string) map[string]string {
	return config.Datadog.GetStringMapString(key)
}
func (c *cfg) GetStringMapStringSlice(key string) map[string][]string {
	return config.Datadog.GetStringMapStringSlice(key)
}
func (c *cfg) GetSizeInBytes(key string) uint {
	return config.Datadog.GetSizeInBytes(key)
}
func (c *cfg) AllSettings() map[string]interface{} {
	return config.Datadog.AllSettings()
}
func (c *cfg) AllSettingsWithoutDefault() map[string]interface{} {
	return config.Datadog.AllSettingsWithoutDefault()
}
func (c *cfg) AllKeys() []string {
	return config.Datadog.AllKeys()
}
func (c *cfg) GetKnownKeys() map[string]interface{} {
	return config.Datadog.GetKnownKeys()
}
func (c *cfg) GetEnvVars() []string {
	return config.Datadog.GetEnvVars()
}
func (c *cfg) IsSectionSet(section string) bool {
	return config.Datadog.IsSectionSet(section)
}
func (c *cfg) Warnings() *config.Warnings {
	return c.warnings
}

func newMock(deps dependencies, t testing.TB) Component {
	old := config.Datadog
	config.Datadog = config.NewConfig("mock", "XXXX", strings.NewReplacer())
	c := &cfg{
		warnings: &config.Warnings{},
	}

	// call InitConfig to set defaults.
	config.InitConfig(config.Datadog)

	// Viper's `GetXxx` methods read environment variables at the time they are
	// called, if those names were passed explicitly to BindEnv*(), so we must
	// also strip all `DD_` environment variables for the duration of the test.
	oldEnv := os.Environ()
	for _, kv := range oldEnv {
		if strings.HasPrefix(kv, "DD_") {
			kvslice := strings.SplitN(kv, "=", 2)
			os.Unsetenv(kvslice[0])
		}
	}
	t.Cleanup(func() {
		for _, kv := range oldEnv {
			kvslice := strings.SplitN(kv, "=", 2)
			os.Setenv(kvslice[0], kvslice[1])
		}
	})

	// swap the existing config back at the end of the test.
	t.Cleanup(func() { config.Datadog = old })

	return c
}

func (c *cfg) Set(key string, value interface{}) {
	config.Datadog.Set(key, value)
}
