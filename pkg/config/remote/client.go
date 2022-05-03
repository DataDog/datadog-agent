// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remote

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Client is a remote-configuration client to obtain configurations from the local API
type Client struct {
	m     sync.Mutex
	ctx   context.Context
	close func()

	agentName    string
	products     []string
	pollInterval time.Duration

	grpc           pbgo.AgentSecureClient
	stateClient    *client.Client
	currentConfigs client.Configs

	lastPollErr error

	apmSamplingUpdates chan []client.ConfigAPMSamling
}

// NewClient creates a new client
func NewClient(agentName string, products []data.Product) (*Client, error) {
	client, err := newClient(agentName, products)
	if err != nil {
		return nil, err
	}
	go client.pollLoop()
	return client, nil
}

func newClient(agentName string, products []data.Product, dialOpts ...grpc.DialOption) (*Client, error) {
	token, err := security.FetchAuthToken()
	if err != nil {
		return nil, errors.Wrap(err, "could not acquire agent auth token")
	}
	ctx, close := context.WithCancel(context.Background())
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	grpcClient, err := agentgrpc.GetDDAgentSecureClient(ctx, dialOpts...)
	if err != nil {
		close()
		return nil, err
	}
	stateClient := client.NewClient(meta.RootsDirector().Last(), data.ProductListToString(products))
	return &Client{
		ctx:                ctx,
		agentName:          agentName,
		products:           data.ProductListToString(products),
		grpc:               grpcClient,
		close:              close,
		pollInterval:       1 * time.Second,
		stateClient:        stateClient,
		apmSamplingUpdates: make(chan []client.ConfigAPMSamling, 8),
	}, nil
}

// Close closes the client
func (c *Client) Close() {
	c.close()
	close(c.apmSamplingUpdates)
}

func (c *Client) pollLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.pollInterval):
			c.lastPollErr = c.poll()
			if c.lastPollErr != nil {
				log.Errorf("could not poll remote-config agent service: %v", c.lastPollErr)
			}
			c.updateConfigs()
		}
	}
}

func (c *Client) poll() error {
	c.m.Lock()
	defer c.m.Unlock()
	state := c.stateClient.State()
	lastPollErr := ""
	if c.lastPollErr != nil {
		lastPollErr = c.lastPollErr.Error()
	}
	response, err := c.grpc.ClientGetConfigs(c.ctx, &pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			Id:      c.stateClient.ID(),
			IsAgent: true,
			ClientAgent: &pbgo.ClientAgent{
				Name:    c.agentName,
				Version: version.AgentVersion,
			},
			State: &pbgo.ClientState{
				RootVersion:    uint64(state.RootVersion),
				TargetsVersion: uint64(state.TargetsVersion),
				ConfigStates:   c.configStates(),
				HasError:       c.lastPollErr != nil,
				Error:          lastPollErr,
			},
			Products: c.products,
		},
	})
	if err != nil {
		return err
	}
	targetFiles := make(map[string][]byte, len(response.TargetFiles))
	for _, targetFile := range response.TargetFiles {
		targetFiles[targetFile.Path] = targetFile.Raw
	}
	return c.stateClient.Update(client.Update{
		Roots:       response.Roots,
		Targets:     response.Targets,
		TargetFiles: targetFiles,
	})
}

func (c *Client) updateConfigs() {
	newConfigs := c.stateClient.GetConfigs(time.Now().Unix())
	updatedProducts := newConfigs.Diff(c.currentConfigs)
	c.currentConfigs = newConfigs
	if updatedProducts.APMSampling {
		c.apmSamplingUpdates <- newConfigs.APMSamplingConfigs
	}
}

func (c *Client) configStates() []*pbgo.ConfigState {
	var configStates []*pbgo.ConfigState
	configs := c.currentConfigs
	for _, config := range configs.APMSamplingConfigs {
		configStates = append(configStates, &pbgo.ConfigState{
			Product: string(data.ProductAPMSampling),
			Id:      config.ID,
			Version: config.Version,
		})
	}
	return configStates
}

// APMSamplingUpdates returns a chan to consume apm sampling updates
func (c *Client) APMSamplingUpdates() <-chan []client.ConfigAPMSamling {
	return c.apmSamplingUpdates
}
