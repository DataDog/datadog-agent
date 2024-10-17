// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configresolver

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/stretchr/testify/assert"

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
	ExtraConfig   map[string]string
}

// Equal returns whether the two dummyService are equal
func (s *dummyService) Equal(o listeners.Service) bool {
	return reflect.DeepEqual(s, o)
}

// GetServiceID returns the service entity name
func (s *dummyService) GetServiceID() string {
	return s.ID
}

// GetADIdentifiers returns dummy identifiers
func (s *dummyService) GetADIdentifiers(context.Context) ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns dummy hosts
func (s *dummyService) GetHosts(context.Context) (map[string]string, error) {
	return s.Hosts, nil
}

// GetPorts returns dummy ports
func (s *dummyService) GetPorts(context.Context) ([]listeners.ContainerPort, error) {
	return s.Ports, nil
}

// GetTags returns static tags
func (s *dummyService) GetTags() ([]string, error) {
	return []string{"foo:bar"}, nil
}

// GetTagsWithCardinality returns the tags for this service
func (s *dummyService) GetTagsWithCardinality(_ string) ([]string, error) {
	return s.GetTags()
}

// GetPid return a dummy pid
func (s *dummyService) GetPid(context.Context) (int, error) {
	return s.Pid, nil
}

// GetHostname return a dummy hostname
func (s *dummyService) GetHostname(context.Context) (string, error) {
	return s.Hostname, nil
}

// IsReady returns if the service is ready
func (s *dummyService) IsReady(context.Context) bool {
	return true
}

// HasFilter returns false
//
//nolint:revive // TODO(AML) Fix revive linter
func (s *dummyService) HasFilter(_ containers.FilterType) bool {
	return false
}

// GetExtraConfig returns extra configuration
func (s *dummyService) GetExtraConfig(key string) (string, error) {
	return s.ExtraConfig[key], nil
}

// FilterConfigs does nothing.
func (s *dummyService) FilterTemplates(map[string]integration.Config) {
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
	t.Setenv("test_envvar_key", "test_value")
	os.Unsetenv("test_envvar_not_set")

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
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
				Instances:     []integration.Data{integration.Data("host: 127.0.0.2\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
				Instances:     []integration.Data{integration.Data("host: 127.0.0.5\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
				Instances:     []integration.Data{integration.Data("host: 127.0.0.3\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
				Instances:     []integration.Data{integration.Data("host: 127.0.0.4\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
				ServiceID:     "a5901276aed1",
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
				Instances:     []integration.Data{integration.Data("port: 3\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
				Instances:     []integration.Data{integration.Data("port: 1\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
				Instances:     []integration.Data{integration.Data("port: 2\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
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
		//// logs config
		{
			testName: "resolve logs config",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{},
				LogsConfig:    integration.Data("host: %%host%%"),
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{},
				ServiceID:     "a5901276aed1",
				LogsConfig:    integration.Data("host: 127.0.0.1\n"),
			},
		},
		{
			testName: "resolve logs config with %%host%% and no host in service",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{},
				LogsConfig:    integration.Data("host: %%host%%"),
				ServiceID:     "a5901276aed1",
			},
			errorString: "no network found for container a5901276aed1, ignoring it",
		},
		//// envvars (metrics check)
		{
			testName: "simple %%env_test_envvar_key%% (metrics check)",
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
				Instances:     []integration.Data{integration.Data("tags:\n- foo:bar\ntest: test_value\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "not found %%env_test_envvar_not_set%% (metrics check)",
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
			testName: "invalid %%env%% (metrics check)",
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
		//// envvars (logs check)
		{
			testName: "simple %%env_test_envvar_key%% (logs check)",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Pid:           1337,
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{},
				LogsConfig:    integration.Data("test: %%env_test_envvar_key%%"),
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{},
				LogsConfig:    integration.Data("test: test_value\n"),
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "not found %%env_test_envvar_not_set%% (logs check)",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Pid:           1337,
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{},
				LogsConfig:    integration.Data("test: %%env_test_envvar_not_set%%"),
			},
			errorString: "failed to retrieve envvar test_envvar_not_set, skipping service a5901276aed1"},
		{
			testName: "invalid %%env%% (logs check)",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Pid:           1337,
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{},
				LogsConfig:    integration.Data("test: %%env%%"),
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
				Instances:     []integration.Data{integration.Data("tags:\n- foo:bar\ntest: imhere\n")},
				ServiceID:     "a5901276aed1",
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
				Name:                    "cpu",
				ADIdentifiers:           []string{"redis"},
				Instances:               []integration.Data{integration.Data("pid: %%pid%%\ntags: [\"foo\"]")},
				IgnoreAutodiscoveryTags: true,
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("pid: 1337\ntags:\n- foo\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		//// unknown tag
		{
			testName: "invalid %%FOO%% tag in metrics check",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%FOO%%")},
			},
			errorString: "unable to add tags for service 'a5901276aed1', err: invalid %%FOO%% tag",
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
				Instances:     []integration.Data{integration.Data("authKey: secret\ntags:\n- foo:bar\nuser: admin\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "database monitoring aurora configuration resolves",
			svc: &dummyService{
				ID:            "dummy",
				ADIdentifiers: []string{"_dbm_aws_aurora"},
				Hosts: map[string]string{
					"": "my-cluster.cluster-123456789012.us-west-2.rds.amazonaws.com",
				},
				Ports:       []listeners.ContainerPort{{Port: 5432, Name: fmt.Sprintf("p%d", 5432)}},
				ExtraConfig: map[string]string{"region": "us-west-2", "dbclusteridentifier": "my-cluster", "managed_authentication_enabled": "true"},
			},
			tpl: integration.Config{
				Name:          "postgres",
				ADIdentifiers: []string{"_dbm_aws_aurora"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\nport: %%port%%\nregion: %%extra_region%%\nmanaged_authentication_enabled: %%extra_managed_authentication_enabled%%\ndbclusteridentifier: %%extra_dbclusteridentifier%%")},
			},
			out: integration.Config{
				Name:          "postgres",
				ADIdentifiers: []string{"_dbm_aws_aurora"},
				Instances:     []integration.Data{integration.Data("dbclusteridentifier: my-cluster\nhost: my-cluster.cluster-123456789012.us-west-2.rds.amazonaws.com\nmanaged_authentication_enabled: true\nport: 5432\nregion: us-west-2\ntags:\n- foo:bar\n")},
				ServiceID:     "dummy",
			},
		},
		{
			testName: "with IgnoreAutodiscoveryTags disabled",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"kube-state-metrics"},
				Hosts:         map[string]string{"pod": "10.3.2.1"},
			},
			tpl: integration.Config{
				Name:                    "ksm",
				ADIdentifiers:           []string{"kube-state-metrics"},
				Instances:               []integration.Data{integration.Data("host: %%host%%")},
				IgnoreAutodiscoveryTags: false,
			},
			out: integration.Config{
				Name:          "ksm",
				ADIdentifiers: []string{"kube-state-metrics"},
				Instances:     []integration.Data{integration.Data("host: 10.3.2.1\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "with IgnoreAutodiscoveryTags enabled",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"kube-state-metrics"},
				Hosts:         map[string]string{"pod": "10.3.2.1"},
			},
			tpl: integration.Config{
				Name:                    "ksm",
				ADIdentifiers:           []string{"kube-state-metrics"},
				Instances:               []integration.Data{integration.Data("host: %%host%%")},
				IgnoreAutodiscoveryTags: true,
			},
			out: integration.Config{
				Name:          "ksm",
				ADIdentifiers: []string{"kube-state-metrics"},
				Instances:     []integration.Data{integration.Data("host: 10.3.2.1\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "extra kube_* config",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				ExtraConfig:   map[string]string{"pod_name": "redis", "namespace": "default", "pod_uid": "05567616-cb47-41ea-af04-295c1297e957"},
			},
			tpl: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("pod_name: %%kube_pod_name%%\npod_namespace: %%kube_namespace%%\npod_uid: %%kube_pod_uid%%")},
			},
			out: integration.Config{
				Name:          "redis",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("pod_name: redis\npod_namespace: default\npod_uid: 05567616-cb47-41ea-af04-295c1297e957\ntags:\n- foo:bar\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "IPv6 %%host%%",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"ipv4": "192.168.0.1", "ipv6": "fd::1"},
				Ports:         newFakeContainerPorts(),
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host_ipv4: %%host_ipv4%%\nhost_ipv6: %%host_ipv6%%\nurl_ipv4: http://%%host_ipv4%%:%%port%%/data\nurl_ipv6: http://%%host_ipv6%%:%%port%%/data")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host_ipv4: 192.168.0.1\nhost_ipv6: fd::1\ntags:\n- foo:bar\nurl_ipv4: http://192.168.0.1:3/data\nurl_ipv6: http://[fd::1]:3/data\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "logs config in yaml from file",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\n")},
				LogsConfig:    integration.Data("logs:\n- log_processing_rules:\n  - name: numbers\n    pattern: ^[0-9]+$\n    type: include_at_match\n  type: docker\n"),
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      names.File,
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\ntags:\n- foo:bar\n")},
				LogsConfig:    integration.Data("logs:\n- log_processing_rules:\n  - name: numbers\n    pattern: ^[0-9]+$\n    type: include_at_match\n  type: docker\n"),
				ServiceID:     "a5901276aed1",
				Source:        "file:/etc/datadog-agent/conf.d/redisdb.d/auto_conf.yaml",
				Provider:      names.File,
			},
		},
		{
			testName: "logs config in json from container",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\n")},
				LogsConfig:    integration.Data(`[{"service":"any_service","source":"any_source","tags":["a","b:d"]}]`),
				Provider:      names.Container,
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\ntags:\n- foo:bar\n")},
				LogsConfig:    integration.Data(`[{"service":"any_service","source":"any_source","tags":["a","b:d"]}]`),
				ServiceID:     "a5901276aed1",
				Provider:      names.Container,
			},
		},
		{
			testName: "logs config in json from kubernetes",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\n")},
				LogsConfig:    integration.Data(`[{"service":"any_service","source":"any_source","tags":["a","b:d"]}]`),
				Provider:      names.Kubernetes,
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\ntags:\n- foo:bar\n")},
				LogsConfig:    integration.Data(`[{"service":"any_service","source":"any_source","tags":["a","b:d"]}]`),
				ServiceID:     "a5901276aed1",
				Provider:      names.Kubernetes,
			},
		},
		{
			testName: "static tags",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\ntags:\n- statictag:TEST")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\ntags:\n- foo:bar\n- statictag:TEST\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "empty key",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\ntags:\n- statictag:TEST\nzemptykey:\n")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\ntags:\n- foo:bar\n- statictag:TEST\nzemptykey: null\n")},
				ServiceID:     "a5901276aed1",
			},
		},
		{
			testName: "%% that is not a template variable delimiter",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\ntags:\n- password:\"complex%%password\"")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\ntags:\n- foo:bar\n- password:\"complex%%password\"\n")},
				ServiceID:     "a5901276aed1",
			},
		},
	}

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

func BenchmarkResolve(b *testing.B) {
	// Prepare envvars for test
	b.Setenv("test_envvar_key", "test_value")
	os.Unsetenv("test_envvar_not_set")

	testCases := []struct {
		testName    string
		tpl         integration.Config
		svc         listeners.Service
		out         integration.Config
		errorString string
	}{
		{
			testName: "simple",
			svc: &dummyService{
				ID:            "a5901276aed1",
				ADIdentifiers: []string{"redis"},
				Hosts:         map[string]string{"bridge": "127.0.0.1"},
				Ports:         newFakeContainerPorts(),
			},
			tpl: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: %%host%%\nport: %%port%%\nports:\n- foo: %%port_foo%%\n- bar: %%port_bar%%\n- baz: %%port_baz%%\ntest: %%env_test_envvar_key%%\nurl: http://%%host%%:%%port%%/data")},
			},
			out: integration.Config{
				Name:          "cpu",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("host: 127.0.0.1\nport: 3\nports:\n- foo: 1\n- bar: 2\n- baz: 3\ntags:\n- foo:bar\ntest: test_value\nurl: http://127.0.0.1:3/data\n")},
				ServiceID:     "a5901276aed1",
			},
		},
	}

	for i, tc := range testCases {
		b.Run(fmt.Sprintf("case %d: %s", i, tc.testName), func(b *testing.B) {
			// Make sure we don't modify the template object
			checksum := tc.tpl.Digest()

			var cfg integration.Config
			var err error
			for i := 0; i < b.N; i++ {
				cfg, err = Resolve(tc.tpl, tc.svc)
			}
			if tc.errorString != "" {
				assert.EqualError(b, err, tc.errorString)
			} else {
				assert.NoError(b, err)
				assert.Equal(b, tc.out, cfg)
				assert.Equal(b, checksum, tc.tpl.Digest())
			}
		})
	}
}
