// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && (linux || windows)
// +build docker
// +build linux windows

package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
)

func TestStats(t *testing.T) {
	apiStats := types.StatsJSON{Stats: types.Stats{MemoryStats: types.MemoryStats{Limit: 512}}}
	cachedStats := types.StatsJSON{Stats: types.Stats{MemoryStats: types.MemoryStats{Limit: 256}}}
	type fields struct {
		lastScrapeTime time.Time
	}
	type args struct {
		containerID   string
		cacheValidity time.Duration
		clientFunc    dockerStatsFunc
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		loadFunc func()
		want     *types.StatsJSON
		wantErr  bool
		wantFunc func() error
	}{
		{
			name: "empty cache, call api, set cache",
			args: args{
				containerID:   "container_id",
				cacheValidity: 5 * time.Second,
				clientFunc:    func(context.Context, string) (*types.StatsJSON, error) { return &apiStats, nil },
			},
			loadFunc: func() {},
			want:     &apiStats,
			wantErr:  false,
			wantFunc: func() error {
				if _, found := cache.Cache.Get("docker-container_id"); !found {
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
				clientFunc: func(context.Context, string) (*types.StatsJSON, error) {
					return nil, errors.New("should use cache")
				},
			},
			loadFunc: func() { cache.Cache.Set("docker-container_id", &cachedStats, statsCacheExpiration) },
			want:     &cachedStats,
			wantErr:  false,
			wantFunc: func() error {
				if _, found := cache.Cache.Get("docker-container_id"); !found {
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
				clientFunc:    func(context.Context, string) (*types.StatsJSON, error) { return &apiStats, nil },
			},
			loadFunc: func() { cache.Cache.Set("docker-container_id", &cachedStats, statsCacheExpiration) },
			want:     &apiStats,
			wantErr:  false,
			wantFunc: func() error {
				if _, found := cache.Cache.Get("docker-container_id"); !found {
					return errors.New("container stats not cached")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &dockerCollector{
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

func Test_convertNetworkStats(t *testing.T) {
	tests := []struct {
		name           string
		input          map[string]types.NetworkStats
		networks       map[string]string
		expectedOutput provider.ContainerNetworkStats
	}{
		{
			name: "basic",
			input: map[string]types.NetworkStats{
				"eth0": {
					RxBytes:   42,
					RxPackets: 43,
					TxBytes:   44,
					TxPackets: 45,
				},
				"eth1": {
					RxBytes:   46,
					RxPackets: 47,
					TxBytes:   48,
					TxPackets: 49,
				},
			},
			expectedOutput: provider.ContainerNetworkStats{
				BytesSent:   util.Float64Ptr(92),
				BytesRcvd:   util.Float64Ptr(88),
				PacketsSent: util.Float64Ptr(94),
				PacketsRcvd: util.Float64Ptr(90),
				Interfaces: map[string]provider.InterfaceNetStats{
					"eth0": {
						BytesSent:   util.Float64Ptr(44),
						BytesRcvd:   util.Float64Ptr(42),
						PacketsSent: util.Float64Ptr(45),
						PacketsRcvd: util.Float64Ptr(43),
					},
					"eth1": {
						BytesSent:   util.Float64Ptr(48),
						BytesRcvd:   util.Float64Ptr(46),
						PacketsSent: util.Float64Ptr(49),
						PacketsRcvd: util.Float64Ptr(47),
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertNetworkStats(test.input))
		})
	}
}
