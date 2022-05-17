// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package client

import (
	"crypto/rand"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/theupdateframework/go-tuf/data"
)

var (
	idSize     = 21
	idAlphabet = []rune("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

// AgentClient is a remote config client used in a downstream process to retrieve
// remote config updates from an Agent.
type AgentClient struct {
	// Client data
	id                string
	supportedProducts []string

	// TUF related data
	roots         [][]byte
	latestTargets *data.Targets

	// Config files
	apmConfigs   map[string]APMSamplingConfig
	cwsDDConfigs map[string]ConfigCWSDD
}

// NewAgentClient creates a new remote config agent Client. It will store updates
func NewAgentClient(embeddedRoot []byte, products []string) *AgentClient {
	return &AgentClient{
		id:                generateID(),
		supportedProducts: products,
		roots:             [][]byte{embeddedRoot},
		apmConfigs:        make(map[string]APMSamplingConfig),
		cwsDDConfigs:      make(map[string]ConfigCWSDD),
	}
}

// Update processes the ClientGetConfigsResponse from the Agent and updates the
// configuration state
func (c *AgentClient) Update(update *pbgo.ClientGetConfigsResponse) error {
	return nil
}

// NewUpdateRequest builds a new request for a config update from the
// agent for this AgentClient
func (c *AgentClient) NewUpdateRequest() (*pbgo.ClientGetConfigsRequest, error) {
	return nil, nil
}

func generateID() string {
	bytes := make([]byte, idSize)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	id := make([]rune, idSize)
	for i := 0; i < idSize; i++ {
		id[i] = idAlphabet[bytes[i]&63]
	}
	return string(id[:idSize])
}
