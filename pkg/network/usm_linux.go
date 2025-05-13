// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
)

// USMProtocolsData encapsulates the protocols data for Linux version of USM.
type USMProtocolsData struct {
	HTTP     map[http.Key]*http.RequestStats
	HTTP2    map[http.Key]*http.RequestStats
	Kafka    map[kafka.Key]*kafka.RequestStats
	Postgres map[postgres.Key]*postgres.RequestStat
	Redis    map[redis.Key]*redis.RequestStats
}

// NewUSMProtocolsData creates a new instance of USMProtocolsData with initialized maps.
func NewUSMProtocolsData() USMProtocolsData {
	return USMProtocolsData{
		HTTP:     make(map[http.Key]*http.RequestStats),
		HTTP2:    make(map[http.Key]*http.RequestStats),
		Kafka:    make(map[kafka.Key]*kafka.RequestStats),
		Postgres: make(map[postgres.Key]*postgres.RequestStat),
		Redis:    make(map[redis.Key]*redis.RequestStats),
	}
}

// Reset clears the maps in USMProtocolsData.
func (o *USMProtocolsData) Reset() {
	if len(o.HTTP) > 0 {
		o.HTTP = make(map[http.Key]*http.RequestStats)
	}
	if len(o.HTTP2) > 0 {
		o.HTTP2 = make(map[http.Key]*http.RequestStats)
	}
	if len(o.Kafka) > 0 {
		o.Kafka = make(map[kafka.Key]*kafka.RequestStats)
	}
	if len(o.Postgres) > 0 {
		o.Postgres = make(map[postgres.Key]*postgres.RequestStat)
	}
	if len(o.Redis) > 0 {
		o.Redis = make(map[redis.Key]*redis.RequestStats)
	}
}
