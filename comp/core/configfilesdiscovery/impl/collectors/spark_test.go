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
		{name: "native local IP", envName: "SPARK_LOCAL_IP", want: true},
		{name: "native public DNS", envName: "SPARK_PUBLIC_DNS", want: true},
		{name: "official image driver memory", envName: "SPARK_DRIVER_MEMORY", want: true},
		{name: "bitnami RPC encryption", envName: "SPARK_RPC_ENCRYPTION_ENABLED", want: true},
		{name: "bitnami keystore path", envName: "SPARK_SSL_KEYSTORE_FILE", want: true},
		{name: "known Bitnami config directory", envName: "SPARK_CONF_DIR", want: true},
		{name: "known Bitnami mode", envName: "SPARK_MODE", want: true},
		{name: "known standalone process identity", envName: "SPARK_IDENT_STRING", want: true},
		{name: "known standalone process priority", envName: "SPARK_NICENESS", want: true},
		{name: "known standalone worker resource", envName: "SPARK_WORKER_MEMORY", want: true},
		{name: "unrelated", envName: "UNREQUESTED"},
		{name: "lowercase", envName: "spark_local_ip"},
		{name: "unknown setting", envName: "SPARK_FUTURE_SAFE_SETTING"},
		{name: "RPC authentication secret", envName: "SPARK_RPC_AUTHENTICATION_SECRET"},
		{name: "SSL keystore password", envName: "SPARK_SSL_KEYSTORE_PASSWORD"},
		{name: "daemon JVM opts", envName: "SPARK_DAEMON_JAVA_OPTS"},
		{name: "driver Java options", envName: "SPARK_DRIVER_EXTRA_JAVA_OPTIONS"},
		{name: "numbered Java option", envName: "SPARK_JAVA_OPT_1"},
		{name: "classpath", envName: "SPARK_EXTRA_CLASSPATH"},
		{name: "custom command", envName: "SPARK_CUSTOM_COMMAND"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, includeSparkEnvVar(tt.envName))
		})
	}
}

func TestSparkCollectorCollectsDriverEnvVars(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"java", sparkStandaloneDriverClass, "worker-url", "user-jar", "application-class"},
		},
		env: map[string]string{
			"SPARK_DRIVER_MEMORY":             "2g",
			"SPARK_LOCAL_IP":                  "192.0.2.4",
			"SPARK_RPC_AUTHENTICATION_SECRET": "must-not-be-forwarded",
			"SPARK_DAEMON_JAVA_OPTS":          "-Dsecret=must-not-be-forwarded",
			"SPARK_DRIVER_EXTRA_JAVA_OPTIONS": "-Dsecret=must-not-be-forwarded",
			"UNREQUESTED":                     "value",
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	requireSparkEnvVarPredicate(t, reader)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_DRIVER_MEMORY", Value: "2g"},
		{Name: "SPARK_LOCAL_IP", Value: "192.0.2.4"},
	}, collected.EnvVars)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, 0, reader.processCommandlineCalls)
}

func TestSparkCollectorFindsLiveDriverAfterRuntimeInspectionError(t *testing.T) {
	runtimeErr := errors.New("inspect unavailable")
	reader := &sparkCollectorTestReader{
		runtimeCommandlineErr: runtimeErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"/opt/spark/bin/spark-class", sparkStandaloneDriverClass, "worker-url", "user-jar", "application-class"}},
		},
		env: map[string]string{"SPARK_LOCAL_DIRS": "/tmp/spark"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	requireSparkEnvVarPredicate(t, reader)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "SPARK_LOCAL_DIRS", Value: "/tmp/spark"}}, collected.EnvVars)
	assert.Equal(t, 1, reader.processCommandlineCalls)
}

func TestSparkCollectorSkipsNonDriverWithoutReadingEnvVars(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"java", "org.apache.spark.deploy.worker.Worker"},
		},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"java", "org.apache.spark.deploy.master.Master"}},
		},
		env: map[string]string{"SPARK_LOCAL_IP": "192.0.2.4"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
	assert.Empty(t, reader.readEnvVarPredicates)
	assert.Equal(t, 1, reader.processCommandlineCalls)
}

func TestSparkCollectorReturnsRuntimeInspectionErrorWithoutLiveDriver(t *testing.T) {
	runtimeErr := errors.New("inspect unavailable")
	reader := &sparkCollectorTestReader{
		runtimeCommandlineErr: runtimeErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"java", "org.apache.spark.deploy.worker.Worker"}},
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.ErrorIs(t, err, runtimeErr)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
	assert.Empty(t, reader.readEnvVarPredicates)
	assert.Equal(t, 1, reader.processCommandlineCalls)
}

func TestSparkCollectorReturnsEnvVarErrorForDriver(t *testing.T) {
	envErr := errors.New("environment unavailable")
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"java", sparkStandaloneDriverClass},
		},
		readEnvVarsErr: envErr,
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.ErrorIs(t, err, envErr)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
	requireSparkEnvVarPredicate(t, reader)
}

func TestSparkCollectorCanCollectFromProcess(t *testing.T) {
	collector := NewSpark()

	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", sparkStandaloneDriverClass},
	}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"/bin/sh", "-c", "/opt/spark/bin/spark-class " + sparkStandaloneDriverClass + " worker-url user-jar application-class"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "org.apache.spark.deploy.worker.Worker"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "com.example.DriverWrapper"},
	}))
}

func TestSparkCollectorSkipsDriverWithoutSelectedEnvVars(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{"java", sparkStandaloneDriverClass},
		},
		env: map[string]string{"UNREQUESTED": "value"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	requireSparkEnvVarPredicate(t, reader)
	assert.Equal(t, configfilesdiscoveryimpl.CollectedConfig{}, collected)
}

type sparkCollectorTestReader struct {
	runtimeCommandline      configfilesdiscoveryimpl.TargetCommandline
	runtimeCommandlineErr   error
	liveProcessCommandlines []configfilesdiscoveryimpl.TargetCommandline
	processCommandlineCalls int
	env                     map[string]string
	readEnvVarsErr          error
	readEnvVarPredicates    []configfilesdiscoveryimpl.ConfigEnvVarPredicate
}

func (r *sparkCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *sparkCollectorTestReader) Close() {}

func (r *sparkCollectorTestReader) ReadFile(context.Context, string) (configfilesdiscoveryimpl.ConfigFile, error) {
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not implemented")
}

func (r *sparkCollectorTestReader) ReadEnvVars(_ context.Context, predicate configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
	r.readEnvVarPredicates = append(r.readEnvVarPredicates, predicate)
	if r.readEnvVarsErr != nil {
		return nil, r.readEnvVarsErr
	}

	env := make(map[string]string)
	for name, value := range r.env {
		if predicate != nil && predicate(name) {
			env[name] = value
		}
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
	r.processCommandlineCalls++
	return r.liveProcessCommandlines
}

func requireSparkEnvVarPredicate(t *testing.T, reader *sparkCollectorTestReader) {
	t.Helper()
	require.Len(t, reader.readEnvVarPredicates, 1)
	require.NotNil(t, reader.readEnvVarPredicates[0])
}
