// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"time"
)

// Endpoint holds all the organization and network parameters to send logs to Datadog.
type Endpoint struct {
	APIKey                  string `mapstructure:"api_key" json:"api_key"`
	Host                    string
	Port                    int
	UseSSL                  bool
	UseCompression          bool `mapstructure:"use_compression" json:"use_compression"`
	CompressionLevel        int  `mapstructure:"compression_level" json:"compression_level"`
	ProxyAddress            string
	ConnectionResetInterval time.Duration

	BackoffFactor    float64
	BackoffBase      float64
	BackoffMax       float64
	RecoveryInterval int
	RecoveryReset    bool
}

// Endpoints holds the main endpoint and additional ones to dualship logs.
type Endpoints struct {
	Main                   Endpoint
	Additionals            []Endpoint
	UseProto               bool
	UseHTTP                bool
	BatchWait              time.Duration
	BatchMaxConcurrentSend int
	BatchMaxSize           int
	BatchMaxContentSize    int
}

// NewEndpoints returns a new endpoints composite with default batching settings
func NewEndpoints(main Endpoint, additionals []Endpoint, useProto bool, useHTTP bool) *Endpoints {
	return &Endpoints{
		Main:                   main,
		Additionals:            additionals,
		UseProto:               useProto,
		UseHTTP:                useHTTP,
		BatchWait:              config.DefaultBatchWait,
		BatchMaxConcurrentSend: config.DefaultBatchMaxConcurrentSend,
		BatchMaxSize:           config.DefaultBatchMaxSize,
		BatchMaxContentSize:    config.DefaultBatchMaxContentSize,
	}
}

// NewEndpointsWithBatchSettings returns a new endpoints composite with non-default batching settings specified
func NewEndpointsWithBatchSettings(main Endpoint, additionals []Endpoint, useProto bool, useHTTP bool, batchWait time.Duration, batchMaxConcurrentSend int, batchMaxSize int, batchMaxContentSize int) *Endpoints {
	return &Endpoints{
		Main:                   main,
		Additionals:            additionals,
		UseProto:               useProto,
		UseHTTP:                useHTTP,
		BatchWait:              batchWait,
		BatchMaxConcurrentSend: batchMaxConcurrentSend,
		BatchMaxSize:           batchMaxSize,
		BatchMaxContentSize:    batchMaxContentSize,
	}
}
