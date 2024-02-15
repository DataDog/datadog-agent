// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package client is a client usable in the agent or an agent sub-process to receive configs from the core
// remoteconfig service.
package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Constraints on the maximum backoff time when errors occur
const (
	maximalMaxBackoffTime = 90 * time.Second
	minBackoffFactor      = 2.0
	recoveryInterval      = 2

	maxMessageSize = 1024 * 1024 * 110 // 110MB, current backend limit
)

// ConfigUpdater defines the interface that an agent client uses to get config updates
// from the core remote-config service
type ConfigUpdater interface {
	ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
}

// Client is a remote-configuration client to obtain configurations from the local API
type Client struct {
	Options

	m sync.Mutex

	ID string

	startupSync sync.Once
	ctx         context.Context
	closeFn     context.CancelFunc

	cwsWorkloads []string

	lastUpdateError   error
	backoffPolicy     backoff.Policy
	backoffErrorCount int

	updater ConfigUpdater

	state *state.Repository

	listeners map[string][]func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))
}

// Options describes the client options
type Options struct {
	isUpdater            bool
	updaterTags          []string
	agentVersion         string
	agentName            string
	products             []string
	directorRootOverride string
	pollInterval         time.Duration
	clusterName          string
	clusterID            string
	skipTufVerification  bool
}

var defaultOptions = Options{pollInterval: 5 * time.Second}

// TokenFetcher defines the callback used to fetch a token
type TokenFetcher func() (string, error)

// agentGRPCConfigFetcher defines how to retrieve config updates over a
// datadog-agent's secure GRPC client
type agentGRPCConfigFetcher struct {
	client           pbgo.AgentSecureClient
	authTokenFetcher func() (string, error)
}

// NewAgentGRPCConfigFetcher returns a gRPC config fetcher using the secure agent client
func NewAgentGRPCConfigFetcher(ipcAddress string, cmdPort string, authTokenFetcher TokenFetcher) (ConfigUpdater, error) {
	c, err := ddgrpc.GetDDAgentSecureClient(context.Background(), ipcAddress, cmdPort, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(maxMessageSize),
	))
	if err != nil {
		return nil, err
	}

	return &agentGRPCConfigFetcher{
		client:           c,
		authTokenFetcher: authTokenFetcher,
	}, nil
}

// ClientGetConfigs implements the ConfigUpdater interface for agentGRPCConfigFetcher
func (g *agentGRPCConfigFetcher) ClientGetConfigs(ctx context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	// When communicating with the core service via grpc, the auth token is handled
	// by the core-agent, which runs independently. It's not guaranteed it starts before us,
	// or that if it restarts that the auth token remains the same. Thus we need to do this every request.
	token, err := g.authTokenFetcher()
	if err != nil {
		return nil, errors.Wrap(err, "could not acquire agent auth token")
	}

	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}

	ctx = metadata.NewOutgoingContext(ctx, md)

	return g.client.ClientGetConfigs(ctx, request)
}

// NewClient creates a new client
func NewClient(updater ConfigUpdater, opts ...func(o *Options)) (*Client, error) {
	return newClient(updater, opts...)
}

// NewGRPCClient creates a new client that retrieves updates over the datadog-agent's secure GRPC client
func NewGRPCClient(ipcAddress string, cmdPort string, authTokenFetcher TokenFetcher, opts ...func(o *Options)) (*Client, error) {
	grpcClient, err := NewAgentGRPCConfigFetcher(ipcAddress, cmdPort, authTokenFetcher)
	if err != nil {
		return nil, err
	}

	return newClient(grpcClient, opts...)
}

// NewUnverifiedGRPCClient creates a new client that does not perform any TUF verification
func NewUnverifiedGRPCClient(ipcAddress string, cmdPort string, authTokenFetcher TokenFetcher, opts ...func(o *Options)) (*Client, error) {
	grpcClient, err := NewAgentGRPCConfigFetcher(ipcAddress, cmdPort, authTokenFetcher)
	if err != nil {
		return nil, err
	}

	opts = append(opts, WithoutTufVerification())
	return newClient(grpcClient, opts...)
}

// WithProducts specifies the product lists
func WithProducts(products ...string) func(opts *Options) {
	return func(opts *Options) {
		opts.products = products
	}
}

// WithPollInterval specifies the polling interval
func WithPollInterval(duration time.Duration) func(opts *Options) {
	return func(opts *Options) { opts.pollInterval = duration }
}

// WithCluster specifies the cluster name and id
func WithCluster(name, id string) func(opts *Options) {
	return func(opts *Options) { opts.clusterName, opts.clusterID = name, id }
}

// WithoutTufVerification disables TUF verification of configs
func WithoutTufVerification() func(opts *Options) {
	return func(opts *Options) { opts.skipTufVerification = true }
}

// WithDirectorRootOverride specifies the director root to
func WithDirectorRootOverride(directorRootOverride string) func(opts *Options) {
	return func(opts *Options) { opts.directorRootOverride = directorRootOverride }
}

// WithAgent specifies the client name and version
func WithAgent(name, version string) func(opts *Options) {
	return func(opts *Options) { opts.agentName, opts.agentVersion = name, version }
}

// WithUpdater specifies that this client is an updater
func WithUpdater(tags ...string) func(opts *Options) {
	return func(opts *Options) {
		opts.isUpdater = true
		opts.updaterTags = tags
	}
}

func newClient(updater ConfigUpdater, opts ...func(opts *Options)) (*Client, error) {
	var options = defaultOptions
	for _, opt := range opts {
		opt(&options)
	}

	var repository *state.Repository
	var err error

	if !options.skipTufVerification {
		repository, err = state.NewRepository(meta.RootsDirector(options.directorRootOverride).Last())
	} else {
		repository, err = state.NewUnverifiedRepository()
	}
	if err != nil {
		return nil, err
	}

	// A backoff is calculated as a range from which a random value will be selected. The formula is as follows.
	//
	// min = pollInterval * 2^<NumErrors> / minBackoffFactor
	// max = min(maxBackoffTime, pollInterval * 2 ^<NumErrors>)
	//
	// The following values mean each range will always be [pollInterval*2^<NumErrors-1>, min(maxBackoffTime, pollInterval*2^<NumErrors>)].
	// Every success will cause numErrors to shrink by 2.
	backoffPolicy := backoff.NewExpBackoffPolicy(minBackoffFactor, options.pollInterval.Seconds(),
		maximalMaxBackoffTime.Seconds(), recoveryInterval, false)

	ctx, cloneFn := context.WithCancel(context.Background())

	return &Client{
		Options:       options,
		ID:            generateID(),
		startupSync:   sync.Once{},
		ctx:           ctx,
		closeFn:       cloneFn,
		cwsWorkloads:  make([]string, 0),
		state:         repository,
		backoffPolicy: backoffPolicy,
		listeners:     make(map[string][]func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))),
		updater:       updater,
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
	c.closeFn()
}

// UpdateApplyStatus updates the config's metadata to reflect its applied status
func (c *Client) UpdateApplyStatus(cfgPath string, status state.ApplyStatus) {
	c.state.UpdateApplyStatus(cfgPath, status)
}

// SetAgentName updates the agent name of the RC client
// should only be used by the fx component
func (c *Client) SetAgentName(agentName string) {
	c.m.Lock()
	defer c.m.Unlock()
	if c.agentName == "unknown" {
		c.agentName = agentName
	}
}

// Subscribe subscribes to config updates of a product.
func (c *Client) Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.m.Lock()
	defer c.m.Unlock()

	// Make sure the product belongs to the list of requested product
	knownProduct := false
	for _, p := range c.products {
		if p == product {
			knownProduct = true
			break
		}
	}
	if !knownProduct {
		c.products = append(c.products, product)
	}

	c.listeners[product] = append(c.listeners[product], fn)
}

// GetConfigs returns the current configs applied of a product.
func (c *Client) GetConfigs(product string) map[string]state.RawConfig {
	c.m.Lock()
	defer c.m.Unlock()
	return c.state.GetConfigs(product)
}

// SetCWSWorkloads updates the list of workloads that needs cws profiles
func (c *Client) SetCWSWorkloads(workloads []string) {
	c.m.Lock()
	defer c.m.Unlock()
	c.cwsWorkloads = workloads
}

func (c *Client) startFn() {
	go c.pollLoop()
}

// pollLoop is the main polling loop of the client.
//
// pollLoop should never be called manually and only be called via the client's `sync.Once`
// structure in startFn.
func (c *Client) pollLoop() {
	successfulFirstRun := false
	// First run
	err := c.update()
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			// Remote Configuration is disabled as the server isn't initialized
			//
			// As this is not a transient error (that would be codes.Unavailable),
			// stop the client: it shouldn't keep contacting a server that doesn't
			// exist.
			log.Debugf("remote configuration isn't enabled, disabling client")
			return
		}

		// As some clients may start before the core-agent server is up, we log the first error
		// as an Info log as the race is expected. If the error persists, we log with error logs
		log.Infof("retrying the first update of remote-config state (%v)", err)
	} else {
		successfulFirstRun = true
	}

	for {
		interval := c.pollInterval + c.backoffPolicy.GetBackoffDuration(c.backoffErrorCount)
		if !successfulFirstRun && interval > time.Second {
			// If we never managed to contact the RC service, we want to retry faster (max every second)
			// to get a first state as soon as possible.
			// Some products may not start correctly without a first state.
			interval = time.Second
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(interval):
			err := c.update()
			if err != nil {
				if status.Code(err) == codes.Unimplemented {
					// Remote Configuration is disabled as the server isn't initialized
					//
					// As this is not a transient error (that would be codes.Unavailable),
					// stop the client: it shouldn't keep contacting a server that doesn't
					// exist.
					log.Debugf("remote configuration isn't enabled, disabling client")
					return
				}

				if !successfulFirstRun {
					// As some clients may start before the core-agent server is up, we log the first error
					// as an Info log as the race is expected. If the error persists, we log with error logs
					log.Infof("retrying the first update of remote-config state (%v)", err)
				} else {
					c.lastUpdateError = err
					c.backoffPolicy.IncError(c.backoffErrorCount)
					log.Errorf("could not update remote-config state: %v", c.lastUpdateError)
				}
			} else {
				c.lastUpdateError = nil
				successfulFirstRun = true
				c.backoffPolicy.DecError(c.backoffErrorCount)
			}
		}
	}
}

// update requests a config updates from the agent via the secure grpc channel and
// applies that update, informing any registered listeners of any config state changes
// that occurred.
func (c *Client) update() error {
	req, err := c.newUpdateRequest()
	if err != nil {
		return err
	}

	response, err := c.updater.ClientGetConfigs(c.ctx, req)
	if err != nil {
		return err
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
	for product, productListeners := range c.listeners {
		if containsProduct(changedProducts, product) {
			for _, listener := range productListeners {
				listener(c.state.GetConfigs(product), c.state.UpdateApplyStatus)
			}
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
				Hash:      hex.EncodeToString(hash),
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
			Id:         f.ID,
			Version:    f.Version,
			Product:    f.Product,
			ApplyState: uint64(f.ApplyStatus.State),
		})
	}

	// Lock for the product list
	c.m.Lock()
	defer c.m.Unlock()
	req := &pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			State: &pbgo.ClientState{
				RootVersion:        uint64(state.RootsVersion),
				TargetsVersion:     uint64(state.TargetsVersion),
				ConfigStates:       pbConfigState,
				HasError:           hasError,
				Error:              errMsg,
				BackendClientState: state.OpaqueBackendState,
			},
			Id:       c.ID,
			Products: c.products,
		},
		CachedTargetFiles: pbCachedFiles,
	}

	switch c.Options.isUpdater {
	case true:
		req.Client.IsUpdater = true
		req.Client.ClientUpdater = &pbgo.ClientUpdater{
			Tags: c.Options.updaterTags,
			Packages: []*pbgo.PackageState{
				{
					Package:       "datadog-agent",
					StableVersion: "7.50.0",
				},
			},
		}
	case false:
		req.Client.IsAgent = true
		req.Client.ClientAgent = &pbgo.ClientAgent{
			Name:         c.agentName,
			Version:      c.agentVersion,
			ClusterName:  c.clusterName,
			ClusterId:    c.clusterID,
			CwsWorkloads: c.cwsWorkloads,
		}
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
