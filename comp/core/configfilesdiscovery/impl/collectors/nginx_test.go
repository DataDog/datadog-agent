// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"errors"
	"testing"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncludeNginxEnvVar(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"NGINX_ENTRYPOINT_QUIET_LOGS", true},
		{"NGINX_ENTRYPOINT_WORKER_PROCESSES_AUTOTUNE", true},
		{"NGINX_WORKER_PROCESSES", true},
		{"NGINX_HTTP_PORT_NUMBER", true},
		{"NGINX_HTTPS_PORT_NUMBER", true},
		{"NGINX_SKIP_SAMPLE_CERTS", true},
		{"NGINX_ENABLE_STREAM", true},
		{"NGINX_ENABLE_ABSOLUTE_REDIRECT", true},
		{"NGINX_ENABLE_PORT_IN_REDIRECT", true},
		{"NGINX_ENVSUBST_TEMPLATE_DIR", false},
		{"NGINX_ENVSUBST_FILTER", false},
		{"NGINX_HTTP_PORT_NUMBER_FILE", false},
		{"NGINX_SERVER_BLOCKS", false},
		{"NGINX_PASSWORD", false},
		{"NGINX_API_TOKEN", false},
		{"NGINX_FUTURE_SETTING", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, includeNginxEnvVar(tt.name))
		})
	}
}

func TestNginxCollectorCollectsSelectedEnvVars(t *testing.T) {
	reader := &nginxCollectorTestReader{env: map[string]string{
		"NGINX_ENTRYPOINT_WORKER_PROCESSES_AUTOTUNE": "1",
		"NGINX_ENABLE_STREAM":                        "yes",
		"NGINX_HTTP_PORT_NUMBER":                     "8080",
		"NGINX_WORKER_PROCESSES":                     "auto",
		"NGINX_ENVSUBST_FILTER":                      "^APP_",
		"NGINX_HTTP_PORT_NUMBER_FILE":                "/run/secrets/http-port",
		"NGINX_PASSWORD":                             "do-not-send",
	}}

	collected, err := NewNginx().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "NGINX_ENABLE_STREAM", Value: "yes"},
		{Name: "NGINX_ENTRYPOINT_WORKER_PROCESSES_AUTOTUNE", Value: "1"},
		{Name: "NGINX_HTTP_PORT_NUMBER", Value: "8080"},
		{Name: "NGINX_WORKER_PROCESSES", Value: "auto"},
	}, collected.EnvVars)
	assert.Empty(t, collected.ConfigFiles)
}

func TestNginxCollectorCanCollectFromProcess(t *testing.T) {
	collector := NewNginx()

	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"nginx", "-g", "daemon off;"}}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"/usr/sbin/nginx", "-g", "daemon off;"}}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"/bin/sh", "-c", "nginx -g 'daemon off;'"}}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"nginx: master process nginx -g daemon off;"}}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"nginx:", "master", "process", "nginx", "-g", "daemon off;"}}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"nginx: worker process"}}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{Args: []string{"nginx-prometheus-exporter"}}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{}))
}

func TestNginxCollectorReturnsEnvReadError(t *testing.T) {
	expectedErr := errors.New("environment unavailable")
	_, err := NewNginx().Collect(context.Background(), &nginxCollectorTestReader{readEnvVarsErr: expectedErr})
	require.ErrorIs(t, err, expectedErr)
}

type nginxCollectorTestReader struct {
	env            map[string]string
	readEnvVarsErr error
}

func (r *nginxCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *nginxCollectorTestReader) Close() {}

func (r *nginxCollectorTestReader) ReadFile(context.Context, configfilesdiscoveryimpl.VerifiedConfigFilePath) (configfilesdiscoveryimpl.ConfigFile, error) {
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not implemented")
}

func (r *nginxCollectorTestReader) ReadMatchingFiles(context.Context, configfilesdiscoveryimpl.ConfigFileSearch, int, configfilesdiscoveryimpl.ConfigFilePathMatcher) ([]configfilesdiscoveryimpl.ConfigFileReadResult, bool, error) {
	return nil, false, errors.New("not implemented")
}

func (r *nginxCollectorTestReader) ReadEnvVars(_ context.Context, predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
	if r.readEnvVarsErr != nil {
		return nil, r.readEnvVarsErr
	}
	env := make(map[string]string)
	for name, value := range r.env {
		if predicate(name) {
			env[name] = value
		}
	}
	return env, nil
}

func (r *nginxCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	return configfilesdiscoveryimpl.TargetCommandline{}, errors.New("not implemented")
}

func (r *nginxCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	return nil
}
