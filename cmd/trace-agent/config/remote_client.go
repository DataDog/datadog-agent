// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func newRemoteClient() (*remoteClient, error) {
	client, err := remote.NewClient(remote.Facts{ID: "trace-agent", Name: "trace-agent", Version: version.AgentVersion}, []data.Product{data.ProductAPMSampling})
	if err != nil {
		return nil, err
	}
	out := make(chan config.SamplingUpdate, 10) // remote.Client uses 8
	go func() {
		for in := range client.APMSamplingUpdates() {
			configs := make(map[string]uint64)
			for _, c := range in.Config.Configs {
				configs[c.ID] = c.Version
			}
			out <- config.SamplingUpdate{
				Configs: configs,
				Rates:   in.Config.Rates,
			}
		}
		close(out)
	}()
	return &remoteClient{
		client: client,
		out:    out,
	}, nil
}

// remoteClient implements config.RemoteClient
type remoteClient struct {
	client *remote.Client
	out    chan config.SamplingUpdate
}

func (rc *remoteClient) Close() {
	rc.client.Close()
}

func (rc *remoteClient) SamplingUpdates() <-chan config.SamplingUpdate {
	return rc.out
}
