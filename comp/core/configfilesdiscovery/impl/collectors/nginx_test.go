// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
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

func TestNginxCollectorCollectsExplicitConfigFile(t *testing.T) {
	reader := &nginxCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args:       []string{"/docker-entrypoint.sh", "nginx", "-c", "/etc/nginx/custom.conf", "-g", "daemon off;"},
			WorkingDir: "/",
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/nginx/custom.conf": {Path: "/etc/nginx/custom.conf", Content: []byte("worker_processes auto;")},
		},
	}

	collected, err := NewNginx().Collect(context.Background(), reader)

	require.NoError(t, err)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, "/etc/nginx/custom.conf", collected.ConfigFiles[0].Path)
	assert.Equal(t, []byte("worker_processes auto;"), collected.ConfigFiles[0].Content)
	assert.Equal(t, agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_UNKNOWN, collected.ConfigFiles[0].PayloadFormat)
}

func TestNginxCollectorCollectsDefaultConfigFileWithoutExplicitArg(t *testing.T) {
	reader := &nginxCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args:       []string{"/docker-entrypoint.sh", "nginx", "-g", "daemon off;"},
			WorkingDir: "/",
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/nginx/nginx.conf": {Path: "/etc/nginx/nginx.conf", Content: []byte("worker_processes auto;")},
		},
	}

	collected, err := NewNginx().Collect(context.Background(), reader)

	require.NoError(t, err)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, "/etc/nginx/nginx.conf", collected.ConfigFiles[0].Path)
}

func TestNginxCollectorCollectsBitnamiDefaultConfigFile(t *testing.T) {
	reader := &nginxCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args:       []string{"/opt/bitnami/scripts/nginx/run.sh"},
			WorkingDir: "/",
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/opt/bitnami/nginx/conf/nginx.conf": {Path: "/opt/bitnami/nginx/conf/nginx.conf", Content: []byte("worker_processes auto;")},
		},
	}

	collected, err := NewNginx().Collect(context.Background(), reader)

	require.NoError(t, err)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, "/opt/bitnami/nginx/conf/nginx.conf", collected.ConfigFiles[0].Path)
}

func TestNginxCollectorSkipsAmbiguousDefaultConfigFiles(t *testing.T) {
	reader := &nginxCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args:       []string{"nginx", "-g", "daemon off;"},
			WorkingDir: "/",
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/nginx/nginx.conf":              {Path: "/etc/nginx/nginx.conf", Content: []byte("a")},
			"/opt/bitnami/nginx/conf/nginx.conf": {Path: "/opt/bitnami/nginx/conf/nginx.conf", Content: []byte("b")},
		},
		env: map[string]string{"NGINX_WORKER_PROCESSES": "auto"},
	}

	collected, err := NewNginx().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "NGINX_WORKER_PROCESSES", Value: "auto"}}, collected.EnvVars)
}

func TestNginxGetConfigArgFromCommandline(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		wantPath string
		wantOK   bool
	}{
		{"explicit -c", []string{"nginx", "-c", "/etc/nginx/custom.conf"}, "/etc/nginx/custom.conf", true},
		{"explicit -c via entrypoint wrapper", []string{"/docker-entrypoint.sh", "nginx", "-c", "/etc/nginx/custom.conf"}, "/etc/nginx/custom.conf", true},
		{"explicit -c via shell wrapper", []string{"/bin/sh", "-c", "nginx -c /etc/nginx/custom.conf"}, "/etc/nginx/custom.conf", true},
		{"no -c flag", []string{"nginx", "-g", "daemon off;"}, "", false},
		{"-c with no value", []string{"nginx", "-c"}, "", false},
		{"not nginx", []string{"redis-server"}, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path, ok := nginxGetConfigArgFromCommandline(tt.args)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantPath, path)
		})
	}
}

func TestNginxMatchesCommandlineAlwaysFalse(t *testing.T) {
	assert.False(t, nginxMatchesCommandline([]string{"nginx", "-c", "/etc/nginx/custom.conf"}))
	assert.False(t, nginxMatchesCommandline([]string{"nginx"}))
	assert.False(t, nginxMatchesCommandline(nil))
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
	env                map[string]string
	readEnvVarsErr     error
	runtimeCommandline configfilesdiscoveryimpl.TargetCommandline
	files              map[string]configfilesdiscoveryimpl.ConfigFile
}

func (r *nginxCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *nginxCollectorTestReader) Close() {}

func (r *nginxCollectorTestReader) ReadFile(_ context.Context, filePath configfilesdiscoveryimpl.VerifiedConfigFilePath) (configfilesdiscoveryimpl.ConfigFile, error) {
	file, ok := r.files[filePath.String()]
	if !ok {
		return configfilesdiscoveryimpl.ConfigFile{}, errors.New("file not found")
	}
	return file, nil
}

// ReadMatchingFiles is not used by the nginx collector.
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
	return r.runtimeCommandline, nil
}

func (r *nginxCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	return nil
}
