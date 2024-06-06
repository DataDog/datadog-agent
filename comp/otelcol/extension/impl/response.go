package impl

type BuildInfoResponse struct {
	agentVersion string `json:"version"`
	agentCommand string `json:"command"`
	agentDesc    string `json:"description"`
}

type ConfigResponse struct {
	customerConfig        string `json:"customer_configuration,omitempty"`
	envConfig             string `json:"environment_configuration,omitempty,omitempty"`
	runtimeOverrideConfig string `json:"runtime_override_configuration,omitempty"`
	runtimeConfig         string `json:"runtime_configuration,omitempty"`
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
	DebugSourceResponse
}
