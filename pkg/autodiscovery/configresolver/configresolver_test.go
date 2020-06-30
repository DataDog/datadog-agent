// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package configresolver

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/containers"

	// we need some valid check in the catalog to run tests
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system"
)

type dummyService struct {
	ID            string
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []listeners.ContainerPort
	Pid           int
	Hostname      string
	CreationTime  integration.CreationTime
	CheckNames    []string
	ExtraConfig   map[string]string
}

// GetEntity returns the service entity name
func (s *dummyService) GetEntity() string {
	return s.ID
}

// GetEntity returns the service entity name
func (s *dummyService) GetTaggerEntity() string {
	return s.ID
}

// GetADIdentifiers returns dummy identifiers
func (s *dummyService) GetADIdentifiers() ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns dummy hosts
func (s *dummyService) GetHosts() (map[string]string, error) {
	return s.Hosts, nil
}

// GetPorts returns dummy ports
func (s *dummyService) GetPorts() ([]listeners.ContainerPort, error) {
	return s.Ports, nil
}

// GetTags returns mil
func (s *dummyService) GetTags() ([]string, error) {
	return nil, nil
}

// GetPid return a dummy pid
func (s *dummyService) GetPid() (int, error) {
	return s.Pid, nil
}

// GetHostname return a dummy hostname
func (s *dummyService) GetHostname() (string, error) {
	return s.Hostname, nil
}

// GetCreationTime return a dummy creation time
func (s *dummyService) GetCreationTime() integration.CreationTime {
	return s.CreationTime
}

// IsReady returns if the service is ready
func (s *dummyService) IsReady() bool {
	return true
}

// GetCheckNames returns slice of check names defined in docker labels
func (s *dummyService) GetCheckNames() []string {
	return s.CheckNames
}

// HasFilter returns false
func (s *dummyService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig returns extra configuration
func (s *dummyService) GetExtraConfig(key []byte) ([]byte, error) {
	return []byte(s.ExtraConfig[string(key)]), nil
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
				Entity:        "a5901276aed1",
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
			errorString: "unable to add tags for service 'a5901276aed1', err: yaml: found character that cannot start any token",
		},
		//// check overrides
		{
			testName: "same check: override check from file",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				CheckNames:    []string{"redis"},
			},
			tpl: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%")},
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      "file",
			},
			errorString: "ignoring config from file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml: another config is defined for the check redis",
		},
		{
			testName: "empty check name defined: override check from file",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				CheckNames:    []string{""},
			},
			tpl: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%")},
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      "file",
			},
			errorString: "ignoring config from file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml: another empty config is defined with the same AD identifier: [redis]",
		},
		{
			testName: "empty check names list defined: override check from file",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				CheckNames:    []string{},
			},
			tpl: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%")},
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      "file",
			},
			errorString: "ignoring config from file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml: another empty config is defined with the same AD identifier: [redis]",
		},
		{
			testName: "different checks: don't override check from file",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				CheckNames:    []string{"tcp_check", "http_check"},
			},
			tpl: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: localhost")},
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      "file",
			},
			out: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: localhost")},
				InitConfig:    integration.Data{},
				Entity:        "a5901276aed1",
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      "file",
			},
		},
		{
			testName: "not annotated: don't override check from file",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				CheckNames:    nil,
			},
			tpl: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: localhost")},
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      "file",
			},
			out: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: localhost")},
				InitConfig:    integration.Data{},
				Entity:        "a5901276aed1",
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      "file",
			},
		},
		{
			testName: "SNMP testing",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"snmp"},
				ExtraConfig:   map[string]string{"user": "admin", "auth_key": "secret"},
			},
			tpl: integration.Config{
				Name:          "device",
				ADIdentifiers: []string{"snmp"},
				Instances:     []integration.Data{integration.Data("user: %%extra_user%%\nauthKey: %%extra_auth_key%%")},
			},
			out: integration.Config{
				Name:          "device",
				ADIdentifiers: []string{"snmp"},
				Instances:     []integration.Data{integration.Data("user: admin\nauthKey: secret")},
				Entity:        "a5901276aed1",
			},
		},
	}
	validTemplates := 0

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, tc.testName), func(t *testing.T) {
			// Make sure we don't modify the template object
			checksum := tc.tpl.Digest()

			cfg, err := Resolve(tc.tpl, tc.svc)
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
