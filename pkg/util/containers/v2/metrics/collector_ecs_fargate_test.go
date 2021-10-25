// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/stretchr/testify/assert"
)

func TestStats(t *testing.T) {
	apiStats := v2.ContainerStats{Memory: v2.MemStats{Limit: 512}}
	cachedStats := v2.ContainerStats{Memory: v2.MemStats{Limit: 256}}
	type fields struct {
		lastScrapeTime time.Time
	}
	type args struct {
		containerID   string
		cacheValidity time.Duration
		clientFunc    ecsStatsFunc
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		loadFunc func()
		want     *v2.ContainerStats
		wantErr  bool
		wantFunc func() error
	}{
		{
			name: "empty cache, call api, set cache",
			args: args{
				containerID:   "container_id",
				cacheValidity: 5 * time.Second,
				clientFunc:    func(context.Context, string) (*v2.ContainerStats, error) { return &apiStats, nil },
			},
			loadFunc: func() {},
			want:     &apiStats,
			wantErr:  false,
			wantFunc: func() error {
				if _, found := cache.Cache.Get("ecs-stats-container_id"); !found {
					return errors.New("container stats not cached")
				}
				return nil
			},
		},
		{
			name: "cache is valid",
			fields: fields{
				lastScrapeTime: time.Now(),
			},
			args: args{
				containerID:   "container_id",
				cacheValidity: 10 * time.Second,
				clientFunc: func(context.Context, string) (*v2.ContainerStats, error) {
					return nil, errors.New("should use cache")
				},
			},
			loadFunc: func() { cache.Cache.Set("ecs-stats-container_id", &cachedStats, statsCacheExpiration) },
			want:     &cachedStats,
			wantErr:  false,
			wantFunc: func() error {
				if _, found := cache.Cache.Get("ecs-stats-container_id"); !found {
					return errors.New("container stats not cached")
				}
				return nil
			},
		},
		{
			name: "cache is populated, but invalid",
			fields: fields{
				lastScrapeTime: time.Now().Add(-30 * time.Second),
			},
			args: args{
				containerID:   "container_id",
				cacheValidity: 10 * time.Second,
				clientFunc:    func(context.Context, string) (*v2.ContainerStats, error) { return &apiStats, nil },
			},
			loadFunc: func() { cache.Cache.Set("ecs-stats-container_id", &cachedStats, statsCacheExpiration) },
			want:     &apiStats,
			wantErr:  false,
			wantFunc: func() error {
				if _, found := cache.Cache.Get("ecs-stats-container_id"); !found {
					return errors.New("container stats not cached")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &ecsFargateCollector{
				lastScrapeTime: tt.fields.lastScrapeTime,
			}

			tt.loadFunc()
			got, err := e.stats(tt.args.containerID, tt.args.cacheValidity, tt.args.clientFunc)

			assert.Equal(t, tt.wantErr, err != nil)
			assert.EqualValues(t, tt.want, got)
			assert.Nil(t, tt.wantFunc())

			cache.Cache.Flush()
		})
	}
}

func TestConvertEcsNetworkStats(t *testing.T) {
	type args struct {
		netStats v2.NetStatsMap
		networks map[string]string
	}
	tests := []struct {
		name string
		args args
		want *ContainerNetworkStats
	}{
		{
			name: "nominal case",
			args: args{
				netStats: v2.NetStatsMap{"eth1": v2.NetStats{RxBytes: 2398415937, RxPackets: 1898631, TxBytes: 1259037719, TxPackets: 428002}},
				networks: map[string]string{},
			},
			want: &ContainerNetworkStats{
				Interfaces:  map[string]InterfaceNetStats{"eth1": {BytesRcvd: floatPtr(2398415937), PacketsRcvd: floatPtr(1898631), BytesSent: floatPtr(1259037719), PacketsSent: floatPtr(428002)}},
				BytesRcvd:   floatPtr(2398415937),
				PacketsRcvd: floatPtr(1898631),
				BytesSent:   floatPtr(1259037719),
				PacketsSent: floatPtr(428002),
			},
		},
		{
			name: "custom interface name",
			args: args{
				netStats: v2.NetStatsMap{"eth1": v2.NetStats{RxBytes: 2398415937, RxPackets: 1898631, TxBytes: 1259037719, TxPackets: 428002}},
				networks: map[string]string{"eth1": "custom_iface"},
			},
			want: &ContainerNetworkStats{
				Interfaces:  map[string]InterfaceNetStats{"custom_iface": {BytesRcvd: floatPtr(2398415937), PacketsRcvd: floatPtr(1898631), BytesSent: floatPtr(1259037719), PacketsSent: floatPtr(428002)}},
				BytesRcvd:   floatPtr(2398415937),
				PacketsRcvd: floatPtr(1898631),
				BytesSent:   floatPtr(1259037719),
				PacketsSent: floatPtr(428002),
			},
		},
		{
			name: "multiple interfaces",
			args: args{
				netStats: v2.NetStatsMap{
					"eth0": v2.NetStats{RxBytes: 2398415937, RxPackets: 1898631, TxBytes: 1259037719, TxPackets: 428002},
					"eth1": v2.NetStats{TxBytes: 2398415937, TxPackets: 1898631, RxBytes: 1259037719, RxPackets: 428002},
				},
				networks: map[string]string{},
			},
			want: &ContainerNetworkStats{
				Interfaces: map[string]InterfaceNetStats{
					"eth0": {BytesRcvd: floatPtr(2398415937), PacketsRcvd: floatPtr(1898631), BytesSent: floatPtr(1259037719), PacketsSent: floatPtr(428002)},
					"eth1": {BytesSent: floatPtr(2398415937), PacketsSent: floatPtr(1898631), BytesRcvd: floatPtr(1259037719), PacketsRcvd: floatPtr(428002)},
				},
				BytesRcvd:   floatPtr(3657453656),
				PacketsRcvd: floatPtr(2326633),
				BytesSent:   floatPtr(3657453656),
				PacketsSent: floatPtr(2326633),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualValues(t, tt.want, convertEcsNetworkStats(tt.args.netStats, tt.args.networks))
		})
	}
}

func TestConvertEcsStats(t *testing.T) {
	constTime := time.Now()
	type args struct {
		ecsStats *v2.ContainerStats
	}
	tests := []struct {
		name string
		args args
		want *ContainerStats
	}{
		{
			name: "nominal case",
			args: args{
				ecsStats: &v2.ContainerStats{
					CPU:    v2.CPUStats{Usage: v2.CPUUsage{Total: 1137691504, Kernelmode: 80000000, Usermode: 810000000}},
					Memory: v2.MemStats{Limit: 9223372036854772000, Usage: 6504448, Details: v2.DetailedMem{RSS: 4669440, Cache: 651264}},
					IO: v2.IOStats{BytesPerDeviceAndKind: []v2.OPStat{
						{
							Major: 202,
							Minor: 26368,
							Kind:  "Read",
							Value: 638976,
						},
						{
							Major: 202,
							Minor: 26368,
							Kind:  "Write",
							Value: 0,
						},
						{
							Major: 202,
							Minor: 26368,
							Kind:  "Sync",
							Value: 638976,
						},
						{
							Major: 202,
							Minor: 26368,
							Kind:  "Async",
							Value: 0,
						},
						{
							Major: 202,
							Minor: 26368,
							Kind:  "Total",
							Value: 638976,
						},
					},
						OPPerDeviceAndKind: []v2.OPStat{
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Read",
								Value: 12,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Write",
								Value: 0,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Sync",
								Value: 12,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Async",
								Value: 0,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Total",
								Value: 12,
							},
						},
					},
				},
			},
			want: &ContainerStats{
				Timestamp: constTime,
				CPU:       &ContainerCPUStats{Total: floatPtr(1137691504), System: floatPtr(80000000), User: floatPtr(810000000)},
				Memory:    &ContainerMemStats{Limit: floatPtr(9223372036854772000), UsageTotal: floatPtr(6504448), RSS: floatPtr(4669440), Cache: floatPtr(651264)},
				IO:        &ContainerIOStats{ReadBytes: floatPtr(638976), WriteBytes: floatPtr(0), ReadOperations: floatPtr(12), WriteOperations: floatPtr(0)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertEcsStats(tt.args.ecsStats)
			got.Timestamp = constTime

			assert.EqualValues(t, tt.want, got)
		})
	}
}

func floatPtr(u uint64) *float64 {
	f := float64(u)
	return &f
}
