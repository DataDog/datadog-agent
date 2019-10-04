// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

// Endpoint holds all the organization and network parameters to send logs to Datadog.
type Endpoint struct {
	APIKey           string `mapstructure:"api_key"`
	Host             string
	Port             int
	UseSSL           bool
	UseCompression   bool
	CompressionLevel int
	ProxyAddress     string
}

// Endpoints holds the main endpoint and additional ones to dualship logs.
type Endpoints struct {
	Main        Endpoint
	Additionals []Endpoint
	UseProto    bool
	UseHTTP     bool
}

// NewEndpoints returns a new endpoints composite.
func NewEndpoints(main Endpoint, additionals []Endpoint, useProto bool, useHTTP bool) *Endpoints {
	return &Endpoints{
		Main:        main,
		Additionals: additionals,
		UseProto:    useProto,
		UseHTTP:     useHTTP,
	}
}
