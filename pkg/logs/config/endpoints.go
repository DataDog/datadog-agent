// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

type Endpoint struct {
	APIKey string `mapstructure:"api_key"`
	Logset string
	Host   string
	Port   int
}

type MainEndpoint struct {
	Endpoint
	UseSSL       bool
	ProxyAddress string
}

// ServerConfig holds the network configuration of the server to send logs to.
type Endpoints struct {
	Main        MainEndpoint
	Additionals []Endpoint
}

// NewEndpoints returns a new server config.
func NewEndpoints(main MainEndpoint, additionals []Endpoint) *Endpoints {
	return &Endpoints{
		Main:        main,
		Additionals: additionals,
	}
}
