// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcclient

var apmTracingFilePath = "/opt/datadog/inject/inject_config.yaml"

// InvalidAPMTracingPayload indicates we received an APM_TRACING payload we were unable to decode
const InvalidAPMTracingPayload = "INVALID_APM_TRACING_PAYLOAD"

// MissingServiceTarget indicates we were missing the service_target field
const MissingServiceTarget = "MISSING_SERVICE_TARGET"

// FileWriteFailure indicates we were unable to write the RC Updates to a local file for use by the injector
const FileWriteFailure = "FILE_WRITE_FAILURE"

type serviceEnvConfig struct {
	Service        string `yaml:"service"`
	Env            string `yaml:"env"`
	TracingEnabled bool   `yaml:"tracing_enabled"`
}

type tracingEnabledConfig struct {
	TracingEnabled    bool               `yaml:"tracing_enabled"`
	Env               string             `yaml:"env"`
	ServiceEnvConfigs []serviceEnvConfig `yaml:"service_env_configs"`
}

type tracingConfigUpdate struct {
	ID            string `json:"id"`
	Revision      int64  `json:"revision"`
	SchemaVersion string `json:"schema_version"`
	Action        string `json:"action"`
	LibConfig     struct {
		ServiceName    string `json:"service_name"`
		Env            string `json:"env"`
		TracingEnabled bool   `json:"tracing_enabled"`
	} `json:"lib_config"`
	ServiceTarget *struct {
		Service string `json:"service"`
		Env     string `json:"env"`
	} `json:"service_target"`
	InfraTarget *struct {
		Tags []string `json:"tags"`
	} `json:"infra_target"`
}
