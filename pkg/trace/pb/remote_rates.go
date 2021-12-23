package pb

// TargetTPS contains the targeted traces per second the agent should try to sample for a particular service and env
type TargetTPS struct {
	Service   string  `msgpack:"0"`
	Env       string  `msgpack:"1"`
	Value     float64 `msgpack:"2"`
	Rank      uint32  `msgpack:"3"`
	Mechanism uint32  `msgpack:"4"`
}

// APMSampling is the list of target tps
type APMSampling struct {
	TargetTps []TargetTPS `msgpack:"0"`
}
