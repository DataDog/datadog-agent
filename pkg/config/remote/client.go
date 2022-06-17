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
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

// Client is a remote-configuration client to obtain configurations from the local API
type Client struct {
	m sync.Mutex

	ID string

	startupSync sync.Once
	ctx         context.Context
	close       context.CancelFunc

	agentName    string
	agentVersion string
	products     []string

	pollInterval    time.Duration
	lastUpdateError error

	state *state.Repository

	grpc pbgo.AgentSecureClient

	// Listeners
	apmListeners []func(update map[string]state.APMSamplingConfig)
	cwsListeners []func(update map[string]state.ConfigCWSDD)
}

// NewClient creates a new client
func NewClient(agentName string, agentVersion string, products []data.Product, pollInterval time.Duration) (*Client, error) {
	ctx, close := context.WithCancel(context.Background())
	grpcClient, err := grpc.GetDDAgentSecureClient(ctx)
	if err != nil {
		close()
		return nil, err
	}
	repository, err := state.NewRepository(meta.RootsDirector().Last())
	if err != nil {
		close()
		return nil, err
	}

	return &Client{
		ID:           generateID(),
		startupSync:  sync.Once{},
		ctx:          ctx,
		close:        close,
		agentName:    agentName,
		products:     data.ProductListToString(products),
		grpc:         grpcClient,
		state:        repository,
		pollInterval: pollInterval,
		apmListeners: make([]func(update map[string]state.APMSamplingConfig), 0),
		cwsListeners: make([]func(update map[string]state.ConfigCWSDD), 0),
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
// pollLoop should never be called manually and only be called via the client's `sync.Once`
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
// that occurred.
func (c *Client) update() error {
	// This client is running, at the moment, in the trace-agent or the security-agent. The auth token is handled
	// by the core-agent, running independently. It's not guaranteed it starts before us, or that if it restarts that
	// the auth token remains the same. Thus we need to do this every request.
	token, err := security.FetchAuthToken()
	if err != nil {
		return errors.Wrap(err, "could not acquire agent auth token")
	}
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx := metadata.NewOutgoingContext(c.ctx, md)

	req, err := c.newUpdateRequest()
	if err != nil {
		return err
	}
	response, err := c.grpc.ClientGetConfigs(ctx, req)
	if err != nil {
		return err
	}
	// If there isn't a new update for us, the TargetFiles field will
	// be nil and we can stop processing this update.
	if response.TargetFiles == nil {
		return nil
	}

	changedProducts, err := c.applyUpdate(response)
	if err != nil {
		return err
	}
	// We don't want to force the products to reload config if nothing changed
	// in the latest update.
	if len(changedProducts) == 0 {
		return nil
	}

	c.m.Lock()
	defer c.m.Unlock()
	if containsProduct(changedProducts, state.ProductAPMSampling) {
		for _, listener := range c.apmListeners {
			listener(c.state.APMConfigs())
		}
	}
	if containsProduct(changedProducts, state.ProductCWSDD) {
		for _, listener := range c.cwsListeners {
			listener(c.state.CWSDDConfigs())
		}
	}

	return nil
}

func containsProduct(products []string, product string) bool {
	for _, p := range products {
		if product == p {
			return true
		}
	}

	return false
}

// RegisterAPMUpdate registers a callback function to be called after a successful client update that will
// contain the current state of the APMSampling product.
func (c *Client) RegisterAPMUpdate(fn func(update map[string]state.APMSamplingConfig)) {
	c.m.Lock()
	defer c.m.Unlock()
	c.apmListeners = append(c.apmListeners, fn)
	fn(c.state.APMConfigs())
}

// RegisterCWSDDUpdate registers a callback function to be called after a successful client update that will
// contain the current state of the CWSDD product.
func (c *Client) RegisterCWSDDUpdate(fn func(update map[string]state.ConfigCWSDD)) {
	c.m.Lock()
	defer c.m.Unlock()
	c.cwsListeners = append(c.cwsListeners, fn)
	fn(c.state.CWSDDConfigs())
}

func (c *Client) applyUpdate(pbUpdate *pbgo.ClientGetConfigsResponse) ([]string, error) {
	fileMap := make(map[string][]byte, len(pbUpdate.TargetFiles))
	for _, f := range pbUpdate.TargetFiles {
		fileMap[f.Path] = f.Raw
	}

	update := state.Update{
		TUFRoots:      pbUpdate.Roots,
		TUFTargets:    pbUpdate.Targets,
		TargetFiles:   fileMap,
		ClientConfigs: pbUpdate.ClientConfigs,
	}

	return c.state.Update(update)
}

// newUpdateRequests builds a new request for the agent based on the current state of the
// remote config repository.
func (c *Client) newUpdateRequest() (*pbgo.ClientGetConfigsRequest, error) {
	state, err := c.state.CurrentState()
	if err != nil {
		return nil, err
	}

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

	return req, nil
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
