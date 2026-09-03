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

func TestIncludeSparkEnvVar(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		want    bool
	}{
		{name: "master mode", envName: "SPARK_MODE", want: true},
		{name: "master host", envName: "SPARK_MASTER_HOST", want: true},
		{name: "master port", envName: "SPARK_MASTER_PORT", want: true},
		{name: "worker memory", envName: "SPARK_WORKER_MEMORY", want: true},
		{name: "local directories", envName: "SPARK_LOCAL_DIRS", want: true},
		{name: "rpc encryption", envName: "SPARK_RPC_ENCRYPTION_ENABLED", want: true},
		{name: "keystore path", envName: "SPARK_SSL_KEYSTORE_FILE", want: true},
		{name: "unrelated", envName: "UNREQUESTED"},
		{name: "lowercase", envName: "spark_master_host"},
		{name: "rpc authentication secret", envName: "SPARK_RPC_AUTHENTICATION_SECRET"},
		{name: "ssl key password", envName: "SPARK_SSL_KEY_PASSWORD"},
		{name: "master options", envName: "SPARK_MASTER_OPTS"},
		{name: "worker options", envName: "SPARK_WORKER_OPTS"},
		{name: "daemon java options", envName: "SPARK_DAEMON_JAVA_OPTS"},
		{name: "daemon classpath", envName: "SPARK_DAEMON_CLASSPATH"},
		{name: "extra arguments", envName: "SPARK_EXTRA_ARGS"},
		{name: "java tool options", envName: "JAVA_TOOL_OPTIONS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, includeSparkEnvVar(tt.envName))
		})
	}
}

func TestSparkCollectorCollectsMasterEnvVars(t *testing.T) {
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MODE":                      "master",
			"SPARK_MASTER_HOST":               "spark-master",
			"SPARK_MASTER_PORT":               "7077",
			"SPARK_DAEMON_MEMORY":             "1g",
			"SPARK_RPC_ENCRYPTION_ENABLED":    "yes",
			"SPARK_SSL_KEYSTORE_FILE":         "/opt/spark/conf/keystore.jks",
			"SPARK_MASTER_OPTS":               "-Dexample.secret=must-not-be-forwarded",
			"SPARK_RPC_AUTHENTICATION_SECRET": "must-not-be-forwarded",
			"UNREQUESTED":                     "value",
		},
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/spark/bin/spark-class", "org.apache.spark.deploy.master.Master"},
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_DAEMON_MEMORY", Value: "1g"},
		{Name: "SPARK_MASTER_HOST", Value: "spark-master"},
		{Name: "SPARK_MASTER_PORT", Value: "7077"},
		{Name: "SPARK_MODE", Value: "master"},
		{Name: "SPARK_RPC_ENCRYPTION_ENABLED", Value: "yes"},
		{Name: "SPARK_SSL_KEYSTORE_FILE", Value: "/opt/spark/conf/keystore.jks"},
	}, collected.EnvVars)
	assert.Empty(t, collected.ConfigFiles)
}

func TestSparkCollectorRecognizesBitnamiMasterModeFallback(t *testing.T) {
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MODE":        "master",
			"SPARK_MASTER_HOST": "spark-master",
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_MASTER_HOST", Value: "spark-master"},
		{Name: "SPARK_MODE", Value: "master"},
	}, collected.EnvVars)
}

func TestSparkCollectorSkipsApacheWorkerCommand(t *testing.T) {
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_WORKER_MEMORY": "2g",
		},
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/spark/bin/spark-class", "org.apache.spark.deploy.worker.Worker"},
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestSparkCollectorSkipsWorkerModeWithoutMasterCommand(t *testing.T) {
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MODE":          "worker",
			"SPARK_WORKER_MEMORY": "2g",
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestSparkCollectorRecognizesApacheMasterCommand(t *testing.T) {
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MASTER_HOST": "spark-master",
			"SPARK_MODE":        "worker",
		},
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/spark/bin/spark-class", "org.apache.spark.deploy.master.Master"},
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_MASTER_HOST", Value: "spark-master"},
		{Name: "SPARK_MODE", Value: "worker"},
	}, collected.EnvVars)
}

func TestSparkCollectorRecognizesShellFormMasterCommand(t *testing.T) {
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MASTER_HOST": "spark-master",
		},
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/bin/sh", "-c", "spark-class org.apache.spark.deploy.master.Master"},
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_MASTER_HOST", Value: "spark-master"},
	}, collected.EnvVars)
}

func TestSparkCollectorRecognizesLiveBitnamiMasterProcess(t *testing.T) {
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MASTER_HOST": "spark-master",
		},
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"/opt/bitnami/scripts/spark/run.sh"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"java", "org.apache.spark.deploy.master.Master"}},
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_MASTER_HOST", Value: "spark-master"},
	}, collected.EnvVars)
}

func TestSparkCollectorFallsBackWhenRuntimeCommandlineReadFails(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MODE":        "master",
			"SPARK_MASTER_HOST": "spark-master",
		},
		runtimeCommandlineErr: expectedErr,
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_MASTER_HOST", Value: "spark-master"},
		{Name: "SPARK_MODE", Value: "master"},
	}, collected.EnvVars)
}

func TestSparkCollectorFallsBackToLiveProcessWhenRuntimeCommandlineReadFails(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &sparkCollectorTestReader{
		env: map[string]string{
			"SPARK_MASTER_HOST": "spark-master",
		},
		runtimeCommandlineErr: expectedErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"java", "org.apache.spark.deploy.master.Master"}},
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_MASTER_HOST", Value: "spark-master"},
	}, collected.EnvVars)
}

func TestSparkCollectorReturnsRuntimeCommandlineErrorsWithoutFallback(t *testing.T) {
	expectedErr := errors.New("command line unavailable")
	reader := &sparkCollectorTestReader{runtimeCommandlineErr: expectedErr}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestSparkCollectorReturnsReaderErrors(t *testing.T) {
	expectedErr := errors.New("env unavailable")
	reader := &sparkCollectorTestReader{readEnvVarsErr: expectedErr}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

func TestSparkCollectorCanCollectFromProcess(t *testing.T) {
	collector := sparkConfigCollector{}

	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"/opt/spark/bin/spark-class", "org.apache.spark.deploy.master.Master"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"/opt/spark/bin/spark-class", "org.apache.spark.deploy.worker.Worker"},
	}))
}

type sparkCollectorTestReader struct {
	env                     map[string]string
	readEnvVarsErr          error
	runtimeCommandline      configfilesdiscoveryimpl.TargetCommandline
	runtimeCommandlineErr   error
	liveProcessCommandlines []configfilesdiscoveryimpl.TargetCommandline
}

func (r *sparkCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *sparkCollectorTestReader) Close() {}

func (r *sparkCollectorTestReader) ReadFile(context.Context, string) (configfilesdiscoveryimpl.ConfigFile, error) {
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not implemented")
}

func (r *sparkCollectorTestReader) ReadEnvVars(_ context.Context, predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
	if r.readEnvVarsErr != nil {
		return nil, r.readEnvVarsErr
	}

	env := make(map[string]string)
	for name, value := range r.env {
		if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
			continue
		}
		if predicate != nil && !predicate(name) {
			continue
		}
		env[name] = value
	}
	return env, nil
}

func (r *sparkCollectorTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	if r.runtimeCommandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.runtimeCommandlineErr
	}
	return r.runtimeCommandline, nil
}

func (r *sparkCollectorTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	return r.liveProcessCommandlines
}
