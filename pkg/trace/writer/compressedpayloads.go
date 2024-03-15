package writer

//go:generate msgp
type TableCompressedPayload struct {
	AgentVersion       string   `msg:"agent_version"`
	HostName           string   `msg:"hostname"`
	Env                string   `msg:"env"`
	TargetTPS          float64  `msg:"target_tps"`
	ErrorTPS           float64  `msg:"error_tps"`
	RareSamplerEnabled float64  `msg:"rare_sampler_enabled"`
	TracerPayloads     [][]byte `msg:"tracer_payloads"`
}
