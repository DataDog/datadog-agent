package autoinstrumentation

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"os"
	"strconv"
)

func securityClientLibraryConfigEnvInjectors() []*EnvInjector {
	var out []*EnvInjector

	for configKey, env := range map[string]string{
		"admission_controller.auto_instrumentation.asm.enabled":     "DD_APPSEC_ENABLED",
		"admission_controller.auto_instrumentation.iast.enabled":    "DD_IAST_ENABLED",
		"admission_controller.auto_instrumentation.asm_sca.enabled": "DD_APPSEC_SCA_ENABLED",
	} {
		if !config.Datadog().IsSet(configKey) {
			continue
		}
		value := config.Datadog().GetBool(configKey)
		out = append(out, &EnvInjector{
			Key:    env,
			Value:  strconv.FormatBool(value),
			Append: AppendKind{Override: true},
		})
	}

	return out
}

/*
config.BindEnv("apm_config.install_id", "DD_INSTRUMENTATION_INSTALL_ID")
config.BindEnv("apm_config.install_type", "DD_INSTRUMENTATION_INSTALL_TYPE")
config.BindEnv("apm_config.install_time", "DD_INSTRUMENTATION_INSTALL_TIME")
 */
const (
	instrumentationInstallTypeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TYPE"
	instrumentationInstallTimeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TIME"
	instrumentationInstallIDEnvVarName   = "DD_INSTRUMENTATION_INSTALL_ID"
)

func passThroughEnvInjector(envVarName string, defaultValue string) *EnvInjector {
	v := os.Getenv(envVarName)
	if v == "" {
		v = defaultValue
	}
	return &EnvInjector{
		Key:    envVarName,
		Value:  v,
		Append: AppendKind{Override: true},
	}
}

func passThroughTelemetryEnvInjectors() []*EnvInjector {
	return []*EnvInjector{
		passThroughEnvInjector(instrumentationInstallTimeEnvVarName, common.ClusterAgentStartTime),
		passThroughEnvInjector(instrumentationInstallIDEnvVarName, ""),
	}
}
