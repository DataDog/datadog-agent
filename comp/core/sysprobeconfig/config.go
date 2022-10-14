// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobeconfig

import (
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/fx"

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
		deps.Params.SysProbeConfFilePath,
	)
	if err != nil {
		return nil, err
	}

	return &cfg{warnings}, nil
}

func (c *cfg) IsSet(key string) bool {
	return config.SystemProbe.IsSet(key)
}
func (c *cfg) Get(key string) interface{} {
	return config.SystemProbe.Get(key)
}
func (c *cfg) GetString(key string) string {
	return config.SystemProbe.GetString(key)
}
func (c *cfg) GetBool(key string) bool {
	return config.SystemProbe.GetBool(key)
}
func (c *cfg) GetInt(key string) int {
	return config.SystemProbe.GetInt(key)
}
func (c *cfg) GetInt32(key string) int32 {
	return config.SystemProbe.GetInt32(key)
}
func (c *cfg) GetInt64(key string) int64 {
	return config.SystemProbe.GetInt64(key)
}
func (c *cfg) GetFloat64(key string) float64 {
	return config.SystemProbe.GetFloat64(key)
}
func (c *cfg) GetTime(key string) time.Time {
	return config.SystemProbe.GetTime(key)
}
func (c *cfg) GetDuration(key string) time.Duration {
	return config.SystemProbe.GetDuration(key)
}
func (c *cfg) GetStringSlice(key string) []string {
	return config.SystemProbe.GetStringSlice(key)
}
func (c *cfg) GetFloat64SliceE(key string) ([]float64, error) {
	return config.SystemProbe.GetFloat64SliceE(key)
}
func (c *cfg) GetStringMap(key string) map[string]interface{} {
	return config.SystemProbe.GetStringMap(key)
}
func (c *cfg) GetStringMapString(key string) map[string]string {
	return config.SystemProbe.GetStringMapString(key)
}
func (c *cfg) GetStringMapStringSlice(key string) map[string][]string {
	return config.SystemProbe.GetStringMapStringSlice(key)
}
func (c *cfg) GetSizeInBytes(key string) uint {
	return config.SystemProbe.GetSizeInBytes(key)
}
func (c *cfg) AllSettings() map[string]interface{} {
	return config.SystemProbe.AllSettings()
}
func (c *cfg) AllSettingsWithoutDefault() map[string]interface{} {
	return config.SystemProbe.AllSettingsWithoutDefault()
}
func (c *cfg) AllKeys() []string {
	return config.SystemProbe.AllKeys()
}
func (c *cfg) GetKnownKeys() map[string]interface{} {
	return config.SystemProbe.GetKnownKeys()
}
func (c *cfg) GetEnvVars() []string {
	return config.SystemProbe.GetEnvVars()
}
func (c *cfg) IsSectionSet(section string) bool {
	return config.SystemProbe.IsSectionSet(section)
}
func (c *cfg) Warnings() *config.Warnings {
	return c.warnings
}

func newMock(deps dependencies, t testing.TB) Component {
	old := config.SystemProbe
	config.SystemProbe = config.NewConfig("mock", "XXXX", strings.NewReplacer())
	c := &cfg{
		warnings: &config.Warnings{},
	}

	// call InitSystemProbeConfig to set defaults.
	config.InitSystemProbeConfig(config.SystemProbe)

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
	t.Cleanup(func() { config.SystemProbe = old })

	return c
}

func (c *cfg) Set(key string, value interface{}) {
	config.SystemProbe.Set(key, value)
}
