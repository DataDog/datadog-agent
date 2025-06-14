// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
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

func (ns *networkState) storeHTTP2Stats(allStats map[http.Key]*http.RequestStats) {
	storeUSMStats[http.Key, *http.RequestStats](
		allStats,
		ns.clients,
		func(c *client) map[http.Key]*http.RequestStats { return c.usmDelta.HTTP2 },
		func(c *client, m map[http.Key]*http.RequestStats) { c.usmDelta.HTTP2 = m },
		func(prev, new *http.RequestStats) { prev.CombineWith(new) },
		ns.maxHTTPStats,
		stateTelemetry.http2StatsDropped.Inc,
	)
}

// storeKafkaStats stores the latest Kafka stats for all clients
func (ns *networkState) storeKafkaStats(allStats map[kafka.Key]*kafka.RequestStats) {
	storeUSMStats[kafka.Key, *kafka.RequestStats](
		allStats,
		ns.clients,
		func(c *client) map[kafka.Key]*kafka.RequestStats { return c.usmDelta.Kafka },
		func(c *client, m map[kafka.Key]*kafka.RequestStats) { c.usmDelta.Kafka = m },
		func(prev, new *kafka.RequestStats) { prev.CombineWith(new) },
		ns.maxKafkaStats,
		stateTelemetry.kafkaStatsDropped.Inc,
	)
}

// storePostgresStats stores the latest Postgres stats for all clients
func (ns *networkState) storePostgresStats(allStats map[postgres.Key]*postgres.RequestStat) {
	storeUSMStats[postgres.Key, *postgres.RequestStat](
		allStats,
		ns.clients,
		func(c *client) map[postgres.Key]*postgres.RequestStat { return c.usmDelta.Postgres },
		func(c *client, m map[postgres.Key]*postgres.RequestStat) { c.usmDelta.Postgres = m },
		func(prev, new *postgres.RequestStat) { prev.CombineWith(new) },
		ns.maxPostgresStats,
		stateTelemetry.postgresStatsDropped.Inc,
	)
}

// storeRedisStats stores the latest Redis stats for all clients
func (ns *networkState) storeRedisStats(allStats map[redis.Key]*redis.RequestStats) {
	storeUSMStats[redis.Key, *redis.RequestStats](
		allStats,
		ns.clients,
		func(c *client) map[redis.Key]*redis.RequestStats { return c.usmDelta.Redis },
		func(c *client, m map[redis.Key]*redis.RequestStats) { c.usmDelta.Redis = m },
		func(prev, new *redis.RequestStats) { prev.CombineWith(new) },
		ns.maxRedisStats,
		stateTelemetry.redisStatsDropped.Inc,
	)
}

// processUSMDelta processes the USM delta for Linux.
func (ns *networkState) processUSMDelta(stats map[protocols.ProtocolType]interface{}) {
	for protocolType, protocolStats := range stats {
		switch protocolType {
		case protocols.HTTP:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTPStats(stats)
		case protocols.Kafka:
			stats := protocolStats.(map[kafka.Key]*kafka.RequestStats)
			ns.storeKafkaStats(stats)
		case protocols.HTTP2:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTP2Stats(stats)
		case protocols.Postgres:
			stats := protocolStats.(map[postgres.Key]*postgres.RequestStat)
			ns.storePostgresStats(stats)
		case protocols.Redis:
			stats := protocolStats.(map[redis.Key]*redis.RequestStats)
			ns.storeRedisStats(stats)
		}
	}
}
