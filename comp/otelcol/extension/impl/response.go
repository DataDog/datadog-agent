package impl

type BuildInfoResponse struct {
	otelAgentVersion string `json:"otel_agent_version"`
	otelAgentCommand string `json:"otel_agent_command"`
	otelAgentDesc    string `json:"otel_agent_description"`
}

type ConfigResponse struct {
	otelCustomerConfig        string `json:"otel_customer_configuration"`
	otelEnvConfig             string `json:"otel_environment_configuration,omitempty"`
	otelRuntimeOverrideConfig string `json:"otel_runtime_override_configuration"`
	otelRuntimeConfig         string `json:"otel_runtime_configuration"`
}

type OTelFlareSource struct {
	url   string `json:"url"`
	crawl bool   `json:"crawl"`
}

type DebugSourceResponse struct {
	sources map[string]OTelFlareSource `json:sources,omitempty`
}

type Response struct {
	BuildInfoResponse
	ConfigResponse
	OTelFlareSource
	DebugSourceResponse
}
