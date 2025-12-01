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
	"crypto/tls"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

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

	// number of update failed update attempts before reporting an error log
	consecutiveFailuresThreshold = 5
)

// ConfigFetcher defines the interface that an agent client uses to get config updates
type ConfigFetcher interface {
	ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
}

// Listener defines the interface of a remote config listener
type Listener interface {
	// OnUpdate is called when new remote configuration data is available for processing.
	// This method is the primary mechanism for delivering configuration updates to listeners.
	//
	// Parameters:
	//   - configs: A map of configuration file paths to their raw configuration data.
	//             The key is the configuration file path/identifier, and the value contains
	//             the raw configuration content that needs to be processed by the listener.
	//   - applyStateCallback: A callback function that must be called by the listener to report
	//                        the success or failure of applying each configuration. This callback
	//                        takes two parameters:
	//                        * cfgPath: The path/identifier of the configuration being reported
	//                        * status: The apply status indicating success, failure, or error details
	//
	// Behavior:
	//   - Called only when there are actual configuration changes to process
	//   - May be skipped if signature verification fails and ShouldIgnoreSignatureExpiration() returns false
	//   - Listeners should process all provided configurations and report their apply status
	//   - The applyStateCallback must be called for proper state tracking and error reporting
	OnUpdate(map[string]state.RawConfig, func(cfgPath string, status state.ApplyStatus))

	// OnStateChange is called when the remote config client's connectivity state changes.
	// The parameter indicates the health state of the remote config service connection:
	//   - true: The client has successfully connected/reconnected to the remote config service
	//   - false: The client has encountered errors and lost connectivity to the remote config service
	//
	// This callback allows listeners to:
	//   - React to service availability changes
	//   - Implement fallback behavior when remote config is unavailable
	//   - Adjust application behavior based on remote config service health
	//
	// Note: OnStateChange is only called after the first successful connection has been established.
	// Initial connection attempts that fail will not trigger this callback until a successful
	// connection is made, after which subsequent failures will trigger OnStateChange(false).
	OnStateChange(bool)

	// ShouldIgnoreSignatureExpiration determines whether this listener should continue to receive
	// configuration updates even when TUF (The Update Framework) signature verification fails
	// due to expired signatures.
	//
	// If it returns true the listener will continue to receive OnUpdate calls
	// even when signatures, are expired.
	//
	// Security Considerations:
	//   - Returning true bypasses an important security mechanism and should be used cautiously
	//   - Only use true for configurations that are not security-sensitive
	//
	// Usage Context:
	//   - Checked before calling OnUpdate when response.ConfigStatus != CONFIG_STATUS_OK
	//   - Allows selective bypassing of signature expiration per listener
	//   - Enables graceful degradation of service when signature infrastructure has issues
	ShouldIgnoreSignatureExpiration() bool
}

// fetchConfigs defines the function that an agent client uses to get config updates
type fetchConfigs func(context.Context, *pbgo.ClientGetConfigsRequest, ...grpc.CallOption) (*pbgo.ClientGetConfigsResponse, error)

// Client is a remote-configuration client to obtain configurations from the local API
type Client struct {
	Options

	m sync.Mutex

	ID string

	startupSync sync.Once
	ctx         context.Context
	closeFn     context.CancelFunc

	lastUpdateError   error
	backoffPolicy     backoff.Policy
	backoffErrorCount int

	configFetcher ConfigFetcher

	state *state.Repository

	listeners map[string][]Listener

	// Elements that can be changed during the execution of listeners
	// They are atomics so that they don't have to share the top-level mutex
	// when in use
	installerState *atomic.Value // *pbgo.ClientUpdater
	cwsWorkloads   *atomic.Value // []string
}

// Options describes the client options
type Options struct {
	isUpdater            bool
	agentVersion         string
	agentName            string
	products             []string
	directorRootOverride string
	site                 string
	pollInterval         time.Duration
	clusterName          string
	clusterID            string
	skipTufVerification  bool
}

var defaultOptions = Options{pollInterval: 5 * time.Second}

// agentGRPCConfigFetcher defines how to retrieve config updates over a
// datadog-agent's secure GRPC client
type agentGRPCConfigFetcher struct {
	authToken    string
	fetchConfigs fetchConfigs
}

// NewAgentGRPCConfigFetcher returns a gRPC config fetcher using the secure agent client
func NewAgentGRPCConfigFetcher(ipcAddress string, cmdPort string, authToken string, tlsConfig *tls.Config) (ConfigFetcher, error) {
	c, err := newAgentGRPCClient(ipcAddress, cmdPort, tlsConfig)
	if err != nil {
		return nil, err
	}

	return &agentGRPCConfigFetcher{
		authToken:    authToken,
		fetchConfigs: c.ClientGetConfigs,
	}, nil
}

// NewMRFAgentGRPCConfigFetcher returns a gRPC config fetcher using the secure agent MRF client
func NewMRFAgentGRPCConfigFetcher(ipcAddress string, cmdPort string, authToken string, tlsConfig *tls.Config) (ConfigFetcher, error) {
	c, err := newAgentGRPCClient(ipcAddress, cmdPort, tlsConfig)
	if err != nil {
		return nil, err
	}

	return &agentGRPCConfigFetcher{
		authToken:    authToken,
		fetchConfigs: c.ClientGetConfigsHA,
	}, nil
}

func newAgentGRPCClient(ipcAddress string, cmdPort string, tlsConfig *tls.Config) (pbgo.AgentSecureClient, error) {
	c, err := ddgrpc.GetDDAgentSecureClient(context.Background(), ipcAddress, cmdPort, tlsConfig, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(maxMessageSize),
	))
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ClientGetConfigs implements the ConfigFetcher interface for agentGRPCConfigFetcher
func (g *agentGRPCConfigFetcher) ClientGetConfigs(ctx context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	md := metadata.MD{
		"authorization": []string{"Bearer " + g.authToken},
	}

	ctx = metadata.NewOutgoingContext(ctx, md)

	return g.fetchConfigs(ctx, request)
}

// NewClient creates a new client
func NewClient(updater ConfigFetcher, opts ...func(o *Options)) (*Client, error) {
	return newClient(updater, opts...)
}

// NewGRPCClient creates a new client that retrieves updates over the datadog-agent's secure GRPC client
func NewGRPCClient(ipcAddress string, cmdPort string, authToken string, tlsConfig *tls.Config, opts ...func(o *Options)) (*Client, error) {
	grpcClient, err := NewAgentGRPCConfigFetcher(ipcAddress, cmdPort, authToken, tlsConfig)
	if err != nil {
		return nil, err
	}

	return newClient(grpcClient, opts...)
}

// NewUnverifiedMRFGRPCClient creates a new client that does not perform any TUF verification and gets failover configs via gRPC
func NewUnverifiedMRFGRPCClient(ipcAddress string, cmdPort string, authToken string, tlsConfig *tls.Config, opts ...func(o *Options)) (*Client, error) {
	grpcClient, err := NewMRFAgentGRPCConfigFetcher(ipcAddress, cmdPort, authToken, tlsConfig)
	if err != nil {
		return nil, err
	}

	opts = append(opts, WithoutTufVerification())
	return newClient(grpcClient, opts...)
}

// NewUnverifiedGRPCClient creates a new client that does not perform any TUF verification
func NewUnverifiedGRPCClient(ipcAddress string, cmdPort string, authToken string, tlsConfig *tls.Config, opts ...func(o *Options)) (*Client, error) {
	grpcClient, err := NewAgentGRPCConfigFetcher(ipcAddress, cmdPort, authToken, tlsConfig)
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
func WithDirectorRootOverride(site string, directorRootOverride string) func(opts *Options) {
	return func(opts *Options) {
		opts.site = site
		opts.directorRootOverride = directorRootOverride
	}
}

// WithAgent specifies the client name and version
func WithAgent(name, version string) func(opts *Options) {
	return func(opts *Options) { opts.agentName, opts.agentVersion = name, version }
}

// WithUpdater specifies that this client is an updater
func WithUpdater() func(opts *Options) {
	return func(opts *Options) {
		opts.isUpdater = true
	}
}

func newClient(cf ConfigFetcher, opts ...func(opts *Options)) (*Client, error) {
	var options = defaultOptions
	for _, opt := range opts {
		opt(&options)
	}

	var repository *state.Repository
	var err error

	if !options.skipTufVerification {
		repository, err = state.NewRepository(meta.RootsDirector(options.site, options.directorRootOverride).Root())
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

	cwsWorkloads := &atomic.Value{}
	cwsWorkloads.Store([]string{})

	installerState := &atomic.Value{}
	installerState.Store(&pbgo.ClientUpdater{})

	return &Client{
		Options:        options,
		ID:             generateID(),
		startupSync:    sync.Once{},
		ctx:            ctx,
		closeFn:        cloneFn,
		cwsWorkloads:   cwsWorkloads,
		installerState: installerState,
		state:          repository,
		backoffPolicy:  backoffPolicy,
		listeners:      make(map[string][]Listener),
		configFetcher:  cf,
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

// GetClientID gets the client ID
func (c *Client) GetClientID() string {
	return c.ID
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

// SubscribeAll subscribes to all events (config updates, state changed, ...)
func (c *Client) SubscribeAll(product string, listener Listener) {
	c.m.Lock()
	defer c.m.Unlock()

	// Make sure the product belongs to the list of requested product
	knownProduct := slices.Contains(c.products, product)
	if !knownProduct {
		c.products = append(c.products, product)
	}

	c.listeners[product] = append(c.listeners[product], listener)
}

// Subscribe subscribes to config updates of a product.
func (c *Client) Subscribe(product string, cb func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.SubscribeAll(product, NewUpdateListener(cb))
}

// SubscribeIgnoreExpiration subscribes to config updates of a product, but ignores the case when signatures have expired.
func (c *Client) SubscribeIgnoreExpiration(product string, cb func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.SubscribeAll(product, NewUpdateListenerIgnoreExpiration(cb))
}

// GetConfigs returns the current configs applied of a product.
func (c *Client) GetConfigs(product string) map[string]state.RawConfig {
	c.m.Lock()
	defer c.m.Unlock()
	return c.state.GetConfigs(product)
}

// SetCWSWorkloads updates the list of workloads that needs cws profiles
func (c *Client) SetCWSWorkloads(workloads []string) {
	c.cwsWorkloads.Store(workloads)
}

// GetInstallerState gets the installer state
func (c *Client) GetInstallerState() *pbgo.ClientUpdater {
	return c.installerState.Load().(*pbgo.ClientUpdater)
}

// SetInstallerState sets the installer state
func (c *Client) SetInstallerState(state *pbgo.ClientUpdater) {
	c.installerState.Store(state)
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
	consecutiveFailures := 0
	logLimit := log.NewLogLimit(5, time.Minute)
	interval := 0 * time.Second

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(interval):
			err := c.update()
			if err != nil {
				consecutiveFailures++
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
					if logLimit.ShouldLog() {
						message := "retrying the first update of remote-config state (%v), consecutive failures: %d"
						if consecutiveFailures >= consecutiveFailuresThreshold {
							log.Errorf(message, err, consecutiveFailures)
						} else {
							log.Infof(message, err, consecutiveFailures)
						}
					}
				} else {
					c.m.Lock()
					for _, productListeners := range c.listeners {
						for _, listener := range productListeners {
							listener.OnStateChange(false)
						}
					}
					c.m.Unlock()

					c.lastUpdateError = err
					c.backoffErrorCount = c.backoffPolicy.IncError(c.backoffErrorCount)
					log.Errorf("could not update remote-config state:%v; consecutive failures:%d", c.lastUpdateError, consecutiveFailures)
				}
			} else {
				log.Debugf("update successful: successful_first_run:%t, consecutive failures:%d", successfulFirstRun, consecutiveFailures)
				if c.lastUpdateError != nil {
					c.m.Lock()
					for _, productListeners := range c.listeners {
						for _, listener := range productListeners {
							listener.OnStateChange(true)
						}
					}
					c.m.Unlock()
				}

				// record and report that the first update was successful
				if !successfulFirstRun {
					log.Infof("first update successful after %d attempts", consecutiveFailures)
				}
				successfulFirstRun = true
				consecutiveFailures = 0

				c.lastUpdateError = nil
				c.backoffErrorCount = c.backoffPolicy.DecError(c.backoffErrorCount)
			}
		}

		// adjust poll interval
		interval = c.pollInterval + c.backoffPolicy.GetBackoffDuration(c.backoffErrorCount)
		if !successfulFirstRun && interval > time.Second {
			// If we never managed to contact the RC service, we want to retry faster (max every second)
			// to get a first state as soon as possible.
			// Some products may not start correctly without a first state.
			interval = time.Second
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

	response, err := c.configFetcher.ClientGetConfigs(c.ctx, req)
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
				if response.ConfigStatus == pbgo.ConfigStatus_CONFIG_STATUS_OK ||
					!listener.ShouldIgnoreSignatureExpiration() {
					listener.OnUpdate(c.state.GetConfigs(product), c.state.UpdateApplyStatus)
				}
			}
		}
	}
	return nil
}

func containsProduct(products []string, product string) bool {
	return slices.Contains(products, product)
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
			ApplyError: f.ApplyStatus.Error,
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
		installerState, ok := c.installerState.Load().(*pbgo.ClientUpdater)
		if !ok {
			return nil, errors.New("could not load installerState")
		}

		req.Client.IsUpdater = true
		req.Client.ClientUpdater = installerState
	case false:
		cwsWorkloads, ok := c.cwsWorkloads.Load().([]string)
		if !ok {
			return nil, errors.New("could not load cwsWorkloads")
		}

		req.Client.IsAgent = true
		req.Client.ClientAgent = &pbgo.ClientAgent{
			Name:         c.agentName,
			Version:      c.agentVersion,
			ClusterName:  c.clusterName,
			ClusterId:    c.clusterID,
			CwsWorkloads: cwsWorkloads,
		}
	}

	return req, nil
}

type listener struct {
	onUpdate               func(map[string]state.RawConfig, func(cfgPath string, status state.ApplyStatus))
	onStateChange          func(bool)
	shouldIgnoreExpiration bool
}

func (l *listener) OnUpdate(configs map[string]state.RawConfig, cb func(cfgPath string, status state.ApplyStatus)) {
	if l.onUpdate != nil {
		l.onUpdate(configs, cb)
	}
}

func (l *listener) OnStateChange(state bool) {
	if l.onStateChange != nil {
		l.onStateChange(state)
	}
}

func (l *listener) ShouldIgnoreSignatureExpiration() bool {
	return l.shouldIgnoreExpiration
}

// NewUpdateListener creates a remote config listener from a update callback
func NewUpdateListener(onUpdate func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) Listener {
	return &listener{onUpdate: onUpdate}
}

// NewUpdateListenerIgnoreExpiration creates a remote config listener that ignores signature expiration
func NewUpdateListenerIgnoreExpiration(onUpdate func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) Listener {
	return &listener{onUpdate: onUpdate, shouldIgnoreExpiration: true}
}

// NewListener creates a remote config listener from a couple of update and state change callbacks
func NewListener(onUpdate func(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)), onStateChange func(bool)) Listener {
	return &listener{onUpdate: onUpdate, onStateChange: onStateChange}
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
