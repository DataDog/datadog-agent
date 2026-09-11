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
	assert.Equal(t, 2, reader.processCommandlineCalls)
}

func TestSparkGetPropertiesFileFromCommandline(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{name: "SparkSubmit class", args: []string{"java", sparkSubmitClass, sparkPropertiesFileOption, "/etc/spark/driver.conf"}, want: "/etc/spark/driver.conf", ok: true},
		{name: "spark-submit script", args: []string{"/opt/spark/bin/spark-submit", "--properties-file=/etc/spark/driver.conf"}, want: "/etc/spark/driver.conf", ok: true},
		{name: "shell form", args: []string{"/bin/sh", "-c", "spark-submit --properties-file /etc/spark/driver.conf app.jar"}, want: "/etc/spark/driver.conf", ok: true},
		{name: "Spark option values before properties file", args: []string{"spark-submit", "--master", "spark://master:7077", "--class", "example.App", "--properties-file", "/etc/spark/driver.conf", "app.jar"}, want: "/etc/spark/driver.conf", ok: true},
		{name: "Spark -c alias before properties file", args: []string{"spark-submit", "-c", "spark.executor.memory=4g", "--properties-file", "/etc/spark/driver.conf", "app.jar"}, want: "/etc/spark/driver.conf", ok: true},
		{name: "Spark driver default class path before properties file", args: []string{"spark-submit", "--driver-default-class-path", "/opt/spark/conf", "--properties-file", "/etc/spark/driver.conf", "app.jar"}, want: "/etc/spark/driver.conf", ok: true},
		{name: "Spark resource option before properties file", args: []string{"spark-submit", "--driver-resource", "gpu.amount=1", "--properties-file", "/etc/spark/driver.conf", "app.jar"}, want: "/etc/spark/driver.conf", ok: true},
		{name: "application argument is ignored", args: []string{"spark-submit", "app.jar", "--properties-file", "/tmp/application.conf"}},
		{name: "application argument after value-taking option is ignored", args: []string{"spark-submit", "--master", "spark://master:7077", "app.jar", "--properties-file", "/tmp/application.conf"}},
		{name: "no properties file", args: []string{"spark-submit", "app.jar"}},
		{name: "non Spark command", args: []string{"java", "com.example.App", sparkPropertiesFileOption, "/etc/app.conf"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sparkGetPropertiesFileFromCommandline(tt.args)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSparkCollectorReadsExplicitPropertiesFile(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", sparkStandaloneDriverClass}},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{{
			Args: []string{"/opt/spark/bin/spark-submit", sparkPropertiesFileOption, "/etc/spark/driver.conf"},
		}},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/spark/driver.conf": {Path: "/etc/spark/driver.conf", Content: []byte("spark.app.name=example\n")},
		},
		env: map[string]string{"SPARK_CONF_DIR": "/opt/spark/conf"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/spark/driver.conf"}, reader.readFileCalls)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigFile{{Path: "/etc/spark/driver.conf", Content: []byte("spark.app.name=example\n"), PayloadFormat: sparkConfigPayloadFormat}}, collected.ConfigFiles)
}

func TestSparkCollectorCollectsSparkSubmitDriverConfigAndEnvVars(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{
			"java",
			"-Ddd.tags=env:test," + sparkDriverRoleTag,
			sparkSubmitClass,
			sparkPropertiesFileOption,
			"/etc/spark/driver.conf",
		}},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/etc/spark/driver.conf": {Path: "/etc/spark/driver.conf", Content: []byte("spark.app.name=submit-driver\n")},
		},
		env: map[string]string{"SPARK_DRIVER_MEMORY": "2g"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "SPARK_DRIVER_MEMORY", Value: "2g"}}, collected.EnvVars)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigFile{{Path: "/etc/spark/driver.conf", Content: []byte("spark.app.name=submit-driver\n"), PayloadFormat: sparkConfigPayloadFormat}}, collected.ConfigFiles)
}

func TestSparkCollectorCollectsEnvWhenExplicitPropertiesFileIsMissing(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{
			"java",
			"-Ddd.tags=env:test," + sparkDriverRoleTag,
			sparkSubmitClass,
			sparkPropertiesFileOption,
			"/etc/spark/missing.conf",
		}},
		env: map[string]string{"SPARK_DRIVER_MEMORY": "2g"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/etc/spark/missing.conf"}, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "SPARK_DRIVER_MEMORY", Value: "2g"}}, collected.EnvVars)
}

func TestSparkCollectorPropagatesCancellationWhenExplicitPropertiesFileReadIsCanceled(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{
			"java",
			"-Ddd.tags=env:test," + sparkDriverRoleTag,
			sparkSubmitClass,
			sparkPropertiesFileOption,
			"/etc/spark/driver.conf",
		}},
		env: map[string]string{"SPARK_DRIVER_MEMORY": "2g"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	collected, err := NewSpark().Collect(ctx, reader)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "SPARK_DRIVER_MEMORY", Value: "2g"}}, collected.EnvVars)
	assert.Empty(t, collected.ConfigFiles)
}

func TestSparkCollectorDoesNotFallbackWhenExplicitPropertiesFileCannotResolve(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", sparkStandaloneDriverClass}},
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{{
			Args: []string{"/opt/spark/bin/spark-submit", sparkPropertiesFileOption, "relative.conf"},
		}},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/opt/custom/spark-conf/spark-defaults.conf": {Path: "/opt/custom/spark-conf/spark-defaults.conf"},
		},
		env: map[string]string{"SPARK_CONF_DIR": "/opt/custom/spark-conf", "SPARK_DRIVER_MEMORY": "2g"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_CONF_DIR", Value: "/opt/custom/spark-conf"},
		{Name: "SPARK_DRIVER_MEMORY", Value: "2g"},
	}, collected.EnvVars)
}

func TestSparkCollectorDoesNotReadSparkConfDirForStandaloneDriver(t *testing.T) {
	const configPath = "/opt/custom/spark-conf/spark-defaults.conf"
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", sparkStandaloneDriverClass}},
		files:              map[string]configfilesdiscoveryimpl.ConfigFile{configPath: {Path: configPath}},
		env:                map[string]string{"SPARK_CONF_DIR": "/opt/custom/spark-conf", "SPARK_DRIVER_MEMORY": "2g"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_CONF_DIR", Value: "/opt/custom/spark-conf"},
		{Name: "SPARK_DRIVER_MEMORY", Value: "2g"},
	}, collected.EnvVars)
}

func TestSparkCollectorReadsSparkConfDirForTaggedSparkSubmitDriver(t *testing.T) {
	const configPath = "/opt/custom/spark-conf/spark-defaults.conf"
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", "-Ddd.tags=" + sparkDriverRoleTag, sparkSubmitClass, "app.jar"}},
		files:              map[string]configfilesdiscoveryimpl.ConfigFile{configPath: {Path: configPath}},
		env:                map[string]string{"SPARK_CONF_DIR": "/opt/custom/spark-conf"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{configPath}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, sparkConfigPayloadFormat, collected.ConfigFiles[0].PayloadFormat)
}

func TestSparkCollectorCollectsEnvWhenSparkConfDirFileIsMissingForTaggedSparkSubmitDriver(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", "-Ddd.tags=" + sparkDriverRoleTag, sparkSubmitClass, "app.jar"}},
		env:                map[string]string{"SPARK_CONF_DIR": "/opt/custom/spark-conf", "SPARK_DRIVER_MEMORY": "2g"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/custom/spark-conf/spark-defaults.conf"}, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_CONF_DIR", Value: "/opt/custom/spark-conf"},
		{Name: "SPARK_DRIVER_MEMORY", Value: "2g"},
	}, collected.EnvVars)
}

func TestSparkCollectorDoesNotReadWorkerDefaultConfigForStandaloneDriver(t *testing.T) {
	const configPath = "/opt/spark/conf/spark-defaults.conf"
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", sparkStandaloneDriverClass}},
		files:              map[string]configfilesdiscoveryimpl.ConfigFile{configPath: {Path: configPath}},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, reader.readFileCalls)
	assert.Empty(t, collected.ConfigFiles)
}

func TestSparkCollectorReadsDefaultConfigForTaggedSparkSubmitWithoutConfigHints(t *testing.T) {
	const configPath = "/opt/spark/conf/spark-defaults.conf"
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{
			"java",
			"-Ddd.tags=env:test," + sparkDriverRoleTag,
			sparkSubmitClass,
			"--master",
			"spark://master:7077",
			"app.jar",
		}},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{configPath: {Path: configPath}},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{configPath}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configPath, collected.ConfigFiles[0].Path)
}

func TestSparkCollectorReadsBitnamiDefaultConfigForTaggedSparkSubmitWhenApachePathIsMissing(t *testing.T) {
	const configPath = "/opt/bitnami/spark/conf/spark-defaults.conf"
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", "-Ddd.tags=" + sparkDriverRoleTag, sparkSubmitClass, "app.jar"}},
		files:              map[string]configfilesdiscoveryimpl.ConfigFile{configPath: {Path: configPath}},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/spark/conf/spark-defaults.conf", configPath}, reader.readFileCalls)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Equal(t, configPath, collected.ConfigFiles[0].Path)
}

func TestSparkCollectorCollectsConfigWhenEnvReadFails(t *testing.T) {
	const configPath = "/opt/spark/conf/spark-defaults.conf"
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{Args: []string{"java", "-Ddd.tags=" + sparkDriverRoleTag, sparkSubmitClass, "app.jar"}},
		files:              map[string]configfilesdiscoveryimpl.ConfigFile{configPath: {Path: configPath}},
		readEnvVarsErr:     errors.New("environment unavailable"),
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	require.Len(t, collected.ConfigFiles, 1)
	assert.Empty(t, collected.EnvVars)
}

func TestSparkCollectorCollectsEnvAfterRuntimeInspectionErrorWithoutConfigFile(t *testing.T) {
	runtimeErr := errors.New("inspect unavailable")
	reader := &sparkCollectorTestReader{
		runtimeCommandlineErr: runtimeErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"/opt/spark/bin/spark-class", sparkStandaloneDriverClass}},
		},
		env: map[string]string{"SPARK_LOCAL_DIRS": "/tmp/spark"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	assert.Empty(t, collected.ConfigFiles)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "SPARK_LOCAL_DIRS", Value: "/tmp/spark"}}, collected.EnvVars)
}

func TestSparkCollectorCollectsSparkSubmitDriverEnvVars(t *testing.T) {
	reader := &sparkCollectorTestReader{
		runtimeCommandline: configfilesdiscoveryimpl.TargetCommandline{
			Args: []string{
				"java",
				"-Ddd.tags=env:staging,spark_process_role:driver,service:spark-job",
				sparkSubmitClass,
			},
		},
		env: map[string]string{
			"SPARK_DRIVER_MEMORY":             "2g",
			"SPARK_RPC_AUTHENTICATION_SECRET": "must-not-be-forwarded",
		},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	requireSparkEnvVarPredicate(t, reader)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{
		{Name: "SPARK_DRIVER_MEMORY", Value: "2g"},
	}, collected.EnvVars)
}

func TestSparkCollectorFindsLiveDriverAfterRuntimeInspectionError(t *testing.T) {
	runtimeErr := errors.New("inspect unavailable")
	reader := &sparkCollectorTestReader{
		runtimeCommandlineErr: runtimeErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{"/opt/spark/bin/spark-class", sparkStandaloneDriverClass, "worker-url", "user-jar", "application-class"}},
		},
		files: map[string]configfilesdiscoveryimpl.ConfigFile{
			"/opt/spark/conf/spark-defaults.conf": {Path: "/opt/spark/conf/spark-defaults.conf"},
		},
		env: map[string]string{"SPARK_LOCAL_DIRS": "/tmp/spark"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	requireSparkEnvVarPredicate(t, reader)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "SPARK_LOCAL_DIRS", Value: "/tmp/spark"}}, collected.EnvVars)
	assert.Empty(t, collected.ConfigFiles)
	assert.Empty(t, reader.readFileCalls)
}

func TestSparkCollectorFindsLiveSparkSubmitDriverAfterRuntimeInspectionError(t *testing.T) {
	runtimeErr := errors.New("inspect unavailable")
	reader := &sparkCollectorTestReader{
		runtimeCommandlineErr: runtimeErr,
		liveProcessCommandlines: []configfilesdiscoveryimpl.TargetCommandline{
			{Args: []string{
				"java",
				"-Ddd.tags=env:staging," + sparkDriverRoleTag,
				sparkSubmitClass,
			}},
		},
		env: map[string]string{"SPARK_LOCAL_DIRS": "/tmp/spark"},
	}

	collected, err := NewSpark().Collect(context.Background(), reader)

	require.NoError(t, err)
	requireSparkEnvVarPredicate(t, reader)
	assert.Equal(t, []configfilesdiscoveryimpl.ConfigEnvVar{{Name: "SPARK_LOCAL_DIRS", Value: "/tmp/spark"}}, collected.EnvVars)
	assert.Equal(t, 4, reader.processCommandlineCalls)
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
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "-Ddd.tags=env:staging," + sparkDriverRoleTag + ",service:spark-job", sparkSubmitClass},
	}))
	assert.True(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"/bin/sh", "-c", "java -Ddd.tags=env:staging," + sparkDriverRoleTag + " " + sparkSubmitClass},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "-Ddd.tags=env:staging", sparkSubmitClass},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "-Ddd.tags=role:driver_helper", sparkSubmitClass},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "-Ddd.tags=spark_process_role:driver-helper", sparkSubmitClass},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{
			"java",
			sparkSubmitClass,
			"--deploy-mode",
			"cluster",
			"--driver-java-options",
			"-Ddd.tags=" + sparkDriverRoleTag,
		},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "-Ddd.tags=" + sparkDriverRoleTag, "org.apache.spark.deploy.worker.Worker"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "org.apache.spark.deploy.worker.Worker"},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", sparkSubmitClass},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "-Ddd.tags=spark_process_role:driver-worker", sparkSubmitClass},
	}))
	assert.False(t, collector.CanCollectFromProcess(configfilesdiscoveryimpl.TargetCommandline{
		Args: []string{"java", "-Ddd.tags=" + sparkDriverRoleTag, "org.apache.spark.deploy.master.Master"},
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
	files                   map[string]configfilesdiscoveryimpl.ConfigFile
	readFileCalls           []string
}

func (r *sparkCollectorTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *sparkCollectorTestReader) Close() {}

func (r *sparkCollectorTestReader) ReadFile(ctx context.Context, configPath string) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, configPath)
	if err := ctx.Err(); err != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, err
	}
	file, ok := r.files[configPath]
	if !ok {
		return configfilesdiscoveryimpl.ConfigFile{}, errors.New("not found")
	}
	return file, nil
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
