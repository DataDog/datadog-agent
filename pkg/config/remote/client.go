// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remote

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

// Client is a remote-configuration client to obtain configurations from the local API
type Client struct {
	ID string

	startupSync sync.Once
	ctx         context.Context
	close       context.CancelFunc

	agentName    string
	agentVersion string
	products     []string

	pollInterval    time.Duration
	lastUpdateError error

	repository *remoteconfig.Repository

	grpc pbgo.AgentSecureClient

	// Listeners
	apmListeners []func(update map[string]remoteconfig.APMSamplingConfig)
	cwsListeners []func(update map[string]remoteconfig.ConfigCWSDD)
}

// NewClient creates a new client
func NewClient(agentName string, agentVersion string, products []data.Product, pollInterval time.Duration) (*Client, error) {
	token, err := security.FetchAuthToken()
	if err != nil {
		return nil, errors.Wrap(err, "could not acquire agent auth token")
	}
	ctx, close := context.WithCancel(context.Background())
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	grpcClient, err := grpc.GetDDAgentSecureClient(ctx)
	if err != nil {
		close()
		return nil, err
	}
	repository := remoteconfig.NewRepository(meta.RootsDirector().Last())
	return &Client{
		ID:           generateID(),
		startupSync:  sync.Once{},
		ctx:          ctx,
		close:        close,
		agentName:    agentName,
		products:     data.ProductListToString(products),
		grpc:         grpcClient,
		repository:   repository,
		pollInterval: 1 * time.Second,
		apmListeners: make([]func(update map[string]remoteconfig.APMSamplingConfig), 0),
		cwsListeners: make([]func(update map[string]remoteconfig.ConfigCWSDD), 0),
	}, nil
}

// Start starts the client's poll loop.
//
// If the client is already started, this is a no-op. At this time, a client that has been stopped cannot
// be restarted.
func (c *Client) Start() {
	c.startupSync.Do(c.startFn)
}

// Close terminates the client's poll loop.
//
// A client that has been closed cannot be restarted
func (c *Client) Close() {
	c.close()
}

func (c *Client) startFn() {
	go c.pollLoop()
}

// pollLoop is the main polling loop of the client.
//
// pollLoop should never be called manaully and only be called via the client's `sync.Once`
// structure in startFn.
func (c *Client) pollLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(c.pollInterval):
			c.lastUpdateError = c.update()
			if c.lastUpdateError != nil {
				log.Errorf("could not update remote-config state: %v", c.lastUpdateError)
			}
		}
	}
}

// update requests a config updates from the agent via the secure grpc channel and
// applies that update, informing any registered listeners of any config state changes
// that occured.
func (c *Client) update() error {
	req := c.newUpdateRequest()
	response, err := c.grpc.ClientGetConfigs(c.ctx, req)
	if err != nil {
		return err
	}

	// If there isn't a new update for us, the TargetFiles field will
	// be nil and we can stop processing this update.
	if response.TargetFiles == nil {
		return nil
	}

	err = c.applyUpdate(response)
	if err != nil {
		return err
	}

	for _, listener := range c.apmListeners {
		listener(c.repository.APMConfigs())
	}

	for _, listener := range c.cwsListeners {
		listener(c.repository.CWSDDConfigs())
	}

	return nil
}

// RegisterAPMUpdate registers a callback function to be called after a successful client update that will
// contain the current state of the APMSampling product.
func (c *Client) RegisterAPMUpdate(fn func(update map[string]remoteconfig.APMSamplingConfig)) {
	c.apmListeners = append(c.apmListeners, fn)
}

// RegisterCWSDDUpdate registers a callback function to be called after a successful client update that will
// contain the current state of the CWSDD product.
func (c *Client) RegisterCWSDDUpdate(fn func(update map[string]remoteconfig.ConfigCWSDD)) {
	c.cwsListeners = append(c.cwsListeners, fn)
}

func (c *Client) applyUpdate(pbUpdate *pbgo.ClientGetConfigsResponse) error {
	fileMap := make(map[string][]byte, len(pbUpdate.TargetFiles))
	for _, f := range pbUpdate.TargetFiles {
		fileMap[f.Path] = f.Raw
	}

	update := remoteconfig.Update{
		TUFRoots:      pbUpdate.Roots,
		TUFTargets:    pbUpdate.Targets,
		TargetFiles:   fileMap,
		ClientConfigs: pbUpdate.ClientConfigs,
	}

	return c.repository.Update(update)
}

// newUpdateRequests builds a new request for the agent based on the current state of the
// remote config repository.
func (c *Client) newUpdateRequest() *pbgo.ClientGetConfigsRequest {
	state := c.repository.CurrentState()

	pbCachedFiles := make([]*pbgo.TargetFileMeta, 0, len(state.CachedFiles))
	for _, f := range state.CachedFiles {
		pbHashes := make([]*pbgo.TargetFileHash, 0, len(f.Hashes))
		for alg, hash := range f.Hashes {
			pbHashes = append(pbHashes, &pbgo.TargetFileHash{
				Algorithm: alg,
				Hash:      hash,
			})
		}
		pbCachedFiles = append(pbCachedFiles, &pbgo.TargetFileMeta{
			Path:   f.Path,
			Length: int64(f.Length),
			Hashes: pbHashes,
		})
	}

	hasError := c.lastUpdateError != nil
	errMsg := ""
	if hasError {
		errMsg = c.lastUpdateError.Error()
	}

	pbConfigState := make([]*pbgo.ConfigState, 0, len(state.Configs))
	for _, f := range state.Configs {
		pbConfigState = append(pbConfigState, &pbgo.ConfigState{
			Id:      f.ID,
			Version: f.Version,
			Product: f.Product,
		})
	}

	req := &pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			State: &pbgo.ClientState{
				RootVersion:    uint64(state.RootsVersion),
				TargetsVersion: uint64(state.TargetsVersion),
				ConfigStates:   pbConfigState,
				HasError:       hasError,
				Error:          errMsg,
			},
			Id:       c.ID,
			Products: c.products,
			IsAgent:  true,
			IsTracer: false,
			ClientAgent: &pbgo.ClientAgent{
				Name:    c.agentName,
				Version: c.agentVersion,
			},
		},
		CachedTargetFiles: pbCachedFiles,
	}

	return req
}

var (
	idSize     = 21
	idAlphabet = []rune("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

// generateID creates a new random ID for a new client instance
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
