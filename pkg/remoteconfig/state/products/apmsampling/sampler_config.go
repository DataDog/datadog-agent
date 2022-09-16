package apmsampling

type SamplerConfig struct {
	AllEnvs SamplerEnvConfig `msgpack:"00"`
	ByEnv   []EnvAndConfig   `msgpack:"01"`
}

type SamplerEnvConfig struct {
	PrioritySamplerTargetTPS *float64 `msgpack:"0"`
	ErrorsSamplerTargetTPS   *float64 `msgpack:"1"`
	RareSamplerEnabled       *bool    `msgpack:"2"`
}

type EnvAndConfig struct {
	Env    string           `msgpack:"0"`
	Config SamplerEnvConfig `msgpack:"1"`
}
