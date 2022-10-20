// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package apmsampling

type SamplerConfig struct {
	AllEnvs SamplerEnvConfig `json:"allEnvs"`
	ByEnv   []EnvAndConfig   `json:"byEnv"`
}

type SamplerEnvConfig struct {
	PrioritySamplerTargetTPS *float64 `json:"prioritySamplerTargetTPS"`
	ErrorsSamplerTargetTPS   *float64 `json:"errorsSamplerTargetTPS"`
	RareSamplerEnabled       *bool    `json:"rareSamplerEnabled"`
}

type EnvAndConfig struct {
	Env    string           `json:"env"`
	Config SamplerEnvConfig `json:"config"`
}
