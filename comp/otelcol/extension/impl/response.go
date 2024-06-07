package impl

type BuildInfoResponse struct {
	AgentVersion string `json:"version"`
	AgentCommand string `json:"command"`
	AgentDesc    string `json:"description"`
}

type ConfigResponse struct {
	CustomerConfig        string `json:"customer_configuration"`
	EnvConfig             string `json:"environment_configuration"`
	RuntimeOverrideConfig string `json:"runtime_override_configuration"`
	RuntimeConfig         string `json:"runtime_configuration"`
}

type OTelFlareSource struct {
	Url   string `json:"url"`
	Crawl bool   `json:"crawl"`
}

type DebugSourceResponse struct {
	Sources map[string]OTelFlareSource `json:"sources,omitempty"`
}

// type Response struct {
// 	BuildInfoResponse   `json:"build_info"`
// 	ConfigResponse      `json:"config"`
// 	DebugSourceResponse `json:"debug,omitempty"`
// }

type Response struct {
	BuildInfoResponse
	ConfigResponse
	DebugSourceResponse
	Environment map[string]string `json:"environment,omitempty"`
}
