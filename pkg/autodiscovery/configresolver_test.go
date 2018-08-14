// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"

	// we need some valid check in the catalog to run tests
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system"
)

func TestParseTemplateVar(t *testing.T) {
	name, key := parseTemplateVar([]byte("%%host%%"))
	assert.Equal(t, "host", string(name))
	assert.Equal(t, "", string(key))

	name, key = parseTemplateVar([]byte("%%host_0%%"))
	assert.Equal(t, "host", string(name))
	assert.Equal(t, "0", string(key))

	name, key = parseTemplateVar([]byte("%%host 0%%"))
	assert.Equal(t, "host0", string(name))
	assert.Equal(t, "", string(key))

	name, key = parseTemplateVar([]byte("%%host_0_1%%"))
	assert.Equal(t, "host", string(name))
	assert.Equal(t, "0_1", string(key))

	name, key = parseTemplateVar([]byte("%%host_network_name%%"))
	assert.Equal(t, "host", string(name))
	assert.Equal(t, "network_name", string(key))
}

func TestGetFallbackHost(t *testing.T) {
	ip, err := getFallbackHost(map[string]string{"bridge": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1"})
	assert.Equal(t, "172.17.0.1", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bridge": "172.17.0.2"})
	assert.Equal(t, "172.17.0.2", ip)
	assert.Equal(t, nil, err)

	ip, err = getFallbackHost(map[string]string{"foo": "172.17.0.1", "bar": "172.17.0.2"})
	assert.Equal(t, "", ip)
	assert.NotNil(t, err)
}

func TestResolve(t *testing.T) {
	// Prepare envvars for test
	err := os.Setenv("test_envvar_key", "test_value")
	require.NoError(t, err)
	os.Unsetenv("test_envvar_not_set")
	defer os.Unsetenv("test_envvar_key")

	testCases := []struct {
		testName    string
		tpl         integration.Config
		svc         listeners.Service
		out         integration.Config
		errorString string
	}{
		//// %%host%% tag testing
		{
			testName: "simple %%host%% with bridge fallback",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1")},
			},
		},
		{
			testName: "simple %%host%% with non-bridge fallback",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"custom": "127.0.0.2"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.2")},
			},
		},
		{
			testName: "%%host_custom%% with two custom networks",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"custom": "127.0.0.5", "other": "127.0.0.3"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host_custom%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.5")},
			},
		},
		{
			testName: "%%host_custom_net%% for docker compose support",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"other": "127.0.0.2", "custom_net": "127.0.0.3"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host_custom_net%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.3")},
			},
		},
		{
			testName: "%%host_custom%% with invalid name, default to fallbackHost",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"other": "127.0.0.4"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host_custom%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.4")},
			},
		},
		{
			testName: "%%host%% with no host in service, error",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%")},
			},
			errorString: "no network found for container a5901276aed1, ignoring it",
		},
		//// %%port%% tag testing
		{
			testName: "simple %%port%%, pick last",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Ports:         newFakeContainerPorts(),
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: %%port%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: 3")},
			},
		},
		{
			testName: "%%port_0%%, pick first",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Ports:         newFakeContainerPorts(),
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: %%port_0%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: 1")},
			},
		},
		{
			testName: "%%port_bar%%, found",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Ports:         newFakeContainerPorts(),
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: %%port_bar%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: 2")},
			},
		},
		{
			testName: "%%port_qux%%, not found, error",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Ports:         newFakeContainerPorts(),
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: %%port_qux%%")},
			},
			errorString: "port qux not found, skipping container a5901276aed1",
		},
		{
			testName: "%%port_4%% too high, error",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Ports:         newFakeContainerPorts(),
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: %%port_4%%")},
			},
			errorString: "index given for the port template var is too big, skipping container a5901276aed1",
		},
		{
			testName: "%%port%% but no port in service, error",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Ports:         []listeners.ContainerPort{},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("port: %%port%%")},
			},
			errorString: "no port found for container a5901276aed1 - ignoring it",
		},
		//// envvars
		{
			testName: "simple %%env_test_envvar_key%%",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Pid:           1337,
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("test: %%env_test_envvar_key%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("test: test_value")},
			},
		},
		{
			testName: "not found %%env_test_envvar_not_set%%",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Pid:           1337,
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("test: %%env_test_envvar_not_set%%")},
			},
			errorString: "failed to retrieve envvar test_envvar_not_set, skipping service a5901276aed1"},
		{
			testName: "invalid %%env%%",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Pid:           1337,
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("test: %%env%%")},
			},
			errorString: "envvar name is missing, skipping service a5901276aed1",
		},
		//// hostname
		{
			testName: "simple %%hostname%%",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hostname:      "imhere",
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("test: %%hostname%%")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("test: imhere")},
			},
		},
		//// other tags testing
		{
			testName: "simple %%pid%%",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Pid:           1337,
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("pid: %%pid%%\ntags: [\"foo\"]")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("pid: 1337\ntags:\n- foo\n")},
			},
		},
		//// unknown tag
		{
			testName: "invalid %%FOO%% tag",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%FOO%%")},
			},
			errorString: "yaml: found character that cannot start any token",
		},
	}
	cr := &ConfigResolver{}
	validTemplates := 0

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, tc.testName), func(t *testing.T) {
			// Make sure we don't modify the template object
			checksum := tc.tpl.Digest()

			cfg, err := cr.resolve(tc.tpl, tc.svc)
			if tc.errorString != "" {
				assert.EqualError(t, err, tc.errorString)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.out, cfg)
				assert.Equal(t, checksum, tc.tpl.Digest())
				validTemplates++
			}
		})
	}
}

func newFakeContainerPorts() []listeners.ContainerPort {
	return []listeners.ContainerPort{
		{Port: 1, Name: "foo"},
		{Port: 2, Name: "bar"},
		{Port: 3, Name: "baz"},
	}
}
