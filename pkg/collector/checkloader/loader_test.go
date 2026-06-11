// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkloader

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

func TestLoadConfigUsesNormalLoaderSelectionRules(t *testing.T) {
	first := &recordingLoader{name: "python", err: errors.New("not mine")}
	second := &recordingLoader{name: "core", check: &testCheck{id: "cpu:first"}}
	loader := New([]check.Loader{first, second}, noopSenderManager{}, nil)
	config := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances: []integration.Data{
			integration.Data("name: first\n"),
		},
	}

	checks, err := loader.LoadConfig(config)

	require.NoError(t, err)
	require.Len(t, checks, 1)
	assert.Equal(t, checkid.ID("cpu:first"), checks[0].ID())
	assert.Empty(t, first.calls)
	assert.Len(t, second.calls, 1)
	assert.Equal(t, config.Instances[0], second.calls[0].instance)
	assert.Equal(t, 0, second.calls[0].instanceIndex)
}

func TestLoadConfigAllowsInstanceLoaderOverride(t *testing.T) {
	initLoader := &recordingLoader{name: "core", check: &testCheck{id: "cpu:core"}}
	instanceLoader := &recordingLoader{name: "python", check: &testCheck{id: "cpu:python"}}
	loader := New([]check.Loader{initLoader, instanceLoader}, noopSenderManager{}, nil)
	config := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances: []integration.Data{
			integration.Data("loader: python\n"),
		},
	}

	checks, err := loader.LoadConfig(config)

	require.NoError(t, err)
	require.Len(t, checks, 1)
	assert.Equal(t, checkid.ID("cpu:python"), checks[0].ID())
	assert.Empty(t, initLoader.calls)
	assert.Len(t, instanceLoader.calls, 1)
}

func TestLoadInstanceUsesDefaultLoaderOrder(t *testing.T) {
	first := &recordingLoader{name: "python", err: errors.New("not mine")}
	second := &recordingLoader{name: "core", check: &testCheck{id: "cpu:first"}}
	recorder := &recordingErrors{}
	loader := New([]check.Loader{first, second}, noopSenderManager{}, recorder)
	config := integration.Config{
		Name:      "cpu",
		Instances: []integration.Data{integration.Data("name: first\n")},
	}

	loaded, ok, err := loader.LoadInstance(noopSenderManager{}, config, config.Instances[0], 0)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, checkid.ID("cpu:first"), loaded.ID())
	assert.Len(t, first.calls, 1)
	assert.Len(t, second.calls, 1)
	assert.Equal(t, []string{"cpu"}, recorder.removed)
	assert.Empty(t, recorder.set)
}

func TestLoadInstanceSkipsJMXInstance(t *testing.T) {
	coreLoader := &recordingLoader{name: "core", check: &testCheck{id: "cpu:first"}}
	loader := New([]check.Loader{coreLoader}, noopSenderManager{}, nil)
	config := integration.Config{
		Name:      "cpu",
		Instances: []integration.Data{integration.Data("loader: jmx\n")},
	}

	loaded, ok, err := loader.LoadInstance(noopSenderManager{}, config, config.Instances[0], 0)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, loaded)
	assert.Empty(t, coreLoader.calls)
}

func TestLoadInstanceRecordsErrorsWhenAllLoadersFail(t *testing.T) {
	first := &recordingLoader{name: "python", err: errors.New("python failed")}
	second := &recordingLoader{name: "core", err: errors.New("core failed")}
	recorder := &recordingErrors{}
	loader := New([]check.Loader{first, second}, noopSenderManager{}, recorder)
	config := integration.Config{
		Name:      "cpu",
		Instances: []integration.Data{integration.Data("name: first\n")},
	}

	loaded, ok, err := loader.LoadInstance(noopSenderManager{}, config, config.Instances[0], 0)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, loaded)
	assert.Equal(t, map[string]string{
		"python": "python failed",
		"core":   "core failed",
	}, recorder.set["cpu"])
	assert.Empty(t, recorder.removed)
}

func TestInitConfigLoader(t *testing.T) {
	loader, err := InitConfigLoader(integration.Data("loader: core\n"))

	require.NoError(t, err)
	assert.Equal(t, "core", loader)
}

func TestInitConfigLoaderReturnsErrorForInvalidYAML(t *testing.T) {
	_, err := InitConfigLoader(integration.Data("loader: ["))

	require.Error(t, err)
}

func TestSelectedInstanceLoaderUsesInitLoaderWhenInstanceLoaderUnset(t *testing.T) {
	loader, err := SelectedInstanceLoader("core", integration.Data("name: first\n"))

	require.NoError(t, err)
	assert.Equal(t, "core", loader)
}

func TestSelectedInstanceLoaderUsesInstanceOverride(t *testing.T) {
	loader, err := SelectedInstanceLoader("core", integration.Data("loader: python\n"))

	require.NoError(t, err)
	assert.Equal(t, "python", loader)
}

func TestSelectedInstanceLoaderReturnsEmptyForDefaultLoaderOrder(t *testing.T) {
	loader, err := SelectedInstanceLoader("", integration.Data("name: first\n"))

	require.NoError(t, err)
	assert.Empty(t, loader)
}

type recordingLoader struct {
	name  string
	check check.Check
	err   error
	calls []loadCall
}

type loadCall struct {
	config        integration.Config
	instance      integration.Data
	instanceIndex int
}

func (l *recordingLoader) Name() string   { return l.name }
func (l *recordingLoader) String() string { return l.name }

func (l *recordingLoader) Load(_ sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (check.Check, error) {
	l.calls = append(l.calls, loadCall{config: config, instance: instance, instanceIndex: instanceIndex})
	if l.err != nil {
		return nil, l.err
	}
	return l.check, nil
}

type recordingErrors struct {
	set     map[string]map[string]string
	removed []string
}

type noopSenderManager struct{}

func (noopSenderManager) GetSender(checkid.ID) (sender.Sender, error) { return nil, nil }
func (noopSenderManager) SetSender(sender.Sender, checkid.ID) error   { return nil }
func (noopSenderManager) DestroySender(checkid.ID)                    {}
func (noopSenderManager) GetDefaultSender() (sender.Sender, error)    { return nil, nil }

func (r *recordingErrors) SetLoaderError(checkName, loaderName, err string) {
	if r.set == nil {
		r.set = make(map[string]map[string]string)
	}
	if r.set[checkName] == nil {
		r.set[checkName] = make(map[string]string)
	}
	r.set[checkName][loaderName] = err
}

func (r *recordingErrors) RemoveLoaderErrors(checkName string) {
	r.removed = append(r.removed, checkName)
}

type testCheck struct {
	id checkid.ID
}

func (c *testCheck) Run() error { return nil }
func (c *testCheck) Stop()      {}
func (c *testCheck) Cancel()    {}
func (c *testCheck) String() string {
	return checkid.IDToCheckName(c.id)
}
func (c *testCheck) Loader() string { return "core" }
func (c *testCheck) Configure(sender.SenderManager, uint64, integration.Data, integration.Data, string, string) error {
	return nil
}
func (c *testCheck) Interval() time.Duration { return time.Second }
func (c *testCheck) ID() checkid.ID          { return c.id }
func (c *testCheck) GetWarnings() []error    { return nil }
func (c *testCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.NewSenderStats(), nil
}
func (c *testCheck) Version() string          { return "" }
func (c *testCheck) ConfigSource() string     { return "" }
func (c *testCheck) ConfigProvider() string   { return "" }
func (c *testCheck) IsTelemetryEnabled() bool { return false }
func (c *testCheck) InitConfig() string       { return "" }
func (c *testCheck) InstanceConfig() string   { return "" }
func (c *testCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}
func (c *testCheck) IsHASupported() bool { return false }
