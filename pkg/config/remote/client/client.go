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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

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
	// wg tracks every goroutine spawned by the client (poll loop + per-listener
	// dispatchers). Close blocks on wg.Wait so callers can rely on "no more
	// listener callbacks fire after Close returns."
	wg sync.WaitGroup

	lastUpdateError   error
	backoffPolicy     backoff.Policy
	backoffErrorCount int

	configFetcher ConfigFetcher

	state *state.Repository

	listeners map[string][]*listenerEntry

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
		listeners:      make(map[string][]*listenerEntry),
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

// Close terminates the client's poll loop and waits for every dispatcher
// goroutine to drain.
//
// After Close returns, no further OnUpdate or OnStateChange callback will
// fire — callers can safely tear down resources their listeners depend on.
// In-flight callbacks at the moment of Close run to completion before Close
// returns; a stuck listener will therefore stall shutdown indefinitely. Use
// CloseTimeout when the caller can't tolerate that — e.g. agent shutdown.
//
// A client that has been closed cannot be restarted.
func (c *Client) Close() {
	log.Infof("RC-ASYNC Close: canceling poll loop and waiting for all workers to drain (unbounded)")
	c.cancelUnderLock()
	start := time.Now()
	c.wg.Wait()
	log.Infof("RC-ASYNC Close: all workers drained in %s", time.Since(start))
}

// CloseTimeout is like Close but bounds how long it will wait for in-flight
// listener callbacks to finish. It returns true if all workers drained within
// the timeout, false if at least one is still running (in which case the
// dispatcher goroutine is leaked — its listener should be considered stuck).
//
// The client context is always canceled before returning, so the poll loop
// itself exits regardless of the result.
func (c *Client) CloseTimeout(timeout time.Duration) bool {
	log.Infof("RC-ASYNC CloseTimeout(%s): canceling poll loop and waiting for workers to drain", timeout)
	c.cancelUnderLock()
	done := make(chan struct{})
	start := time.Now()
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Infof("RC-ASYNC CloseTimeout: all workers drained in %s", time.Since(start))
		return true
	case <-time.After(timeout):
		log.Infof("RC-ASYNC CloseTimeout: TIMED OUT after %s — at least one listener is stuck; leaking its worker", timeout)
		return false
	}
}

// cancelUnderLock cancels the client context with c.m held. Holding the lock
// here is what makes SubscribeAll's "ctx.Err() != nil ⇒ skip wg.Add" check
// race-free: any SubscribeAll that observes a nil ctx error under c.m is
// guaranteed to complete its wg.Add(1) before this function (and therefore
// before wg.Wait) runs, so sync.WaitGroup's "Add concurrent with Wait when
// counter is zero" panic is impossible.
func (c *Client) cancelUnderLock() {
	c.m.Lock()
	c.closeFn()
	c.m.Unlock()
}

// GetClientID gets the client ID
func (c *Client) GetClientID() string {
	return c.ID
}

// UpdateApplyStatus updates the config's metadata to reflect its applied status.
//
// This is the non-version-checked variant: it writes by path only. Listeners
// reacting to an update should prefer the apply-status callback passed into
// OnUpdate, which is version-bound to the snapshot they received and therefore
// safe against a newer config version landing while they process — see
// Client.boundApplyStatus and state.Repository.UpdateApplyStatusIfVersion.
func (c *Client) UpdateApplyStatus(cfgPath string, status state.ApplyStatus) {
	c.state.UpdateApplyStatus(cfgPath, status)
}

// UpdateApplyStatusIfVersion is the version-checked counterpart of
// UpdateApplyStatus, for callers that ack outside of the OnUpdate callback
// (e.g. a separate load/apply cycle that already holds the config's version).
// It writes the status only if the repository still holds expectedVersion for
// cfgPath, and drops the write otherwise so a stale ack can't land on a newer
// version. Returns true if the status was applied.
//
// Listeners that ack from within OnUpdate should instead use the callback they
// are handed — it is already bound to the snapshot's versions.
func (c *Client) UpdateApplyStatusIfVersion(cfgPath string, expectedVersion uint64, status state.ApplyStatus) bool {
	return c.state.UpdateApplyStatusIfVersion(cfgPath, expectedVersion, status)
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

// SubscribeAll subscribes to all events (config updates, state changed, ...).
//
// Each subscription gets its own dispatcher goroutine. OnUpdate / OnStateChange
// calls into the listener are serialized per-listener but run independently of
// the poll loop and of other listeners — a slow listener cannot delay polling
// or block other subscribers. Signals are coalescing (cap=1): if multiple poll
// iterations arrive before the listener drains, only the freshest snapshot is
// delivered.
func (c *Client) SubscribeAll(product string, listener Listener) {
	c.m.Lock()
	defer c.m.Unlock()

	// Race-free shutdown gate: Close cancels c.ctx while holding c.m (see
	// cancelUnderLock). Because this check and the c.wg.Add(1) below also run
	// under c.m, either:
	//   (a) we observe a nil ctx error here and complete wg.Add before Close
	//       can begin wg.Wait, or
	//   (b) Close already canceled the ctx, we observe the error, and skip
	//       wg.Add entirely.
	// Without this serialization, wg.Add concurrent with wg.Wait at counter=0
	// would panic. Log loudly because subscribing after Close is almost
	// always a caller lifecycle bug.
	if c.ctx.Err() != nil {
		log.Warnf("remote-config: SubscribeAll(product=%s) called after Close; subscription dropped", product)
		return
	}

	// Make sure the product belongs to the list of requested product
	knownProduct := slices.Contains(c.products, product)
	if !knownProduct {
		c.products = append(c.products, product)
	}

	entry := &listenerEntry{
		listener: listener,
		product:  product,
		wake:     make(chan struct{}, 1),
	}
	// The worker is tied to the client's ctx, so Close() cancellation tears it
	// down. Spawning here (rather than at Start) means subscribers that register
	// before Start still get a worker — wake signals just queue until update()
	// fires for the first time.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		entry.run(c.ctx)
	}()
	c.listeners[product] = append(c.listeners[product], entry)
	log.Infof("RC-ASYNC SubscribeAll product=%s (%T) — worker spawned, now %d listener(s) for this product", product, listener, len(c.listeners[product]))
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
	// Guard wg.Add against a Close that races Start: like SubscribeAll, the
	// check and the Add run under c.m, and cancelUnderLock cancels under c.m.
	// So either we Add before Close begins wg.Wait, or we observe the canceled
	// ctx and skip starting the poll loop entirely — never wg.Add concurrent
	// with wg.Wait at counter zero (which panics).
	c.m.Lock()
	defer c.m.Unlock()
	if c.ctx.Err() != nil {
		log.Warnf("remote-config: Start called after Close; poll loop not started")
		return
	}
	log.Infof("RC-ASYNC starting poll loop: client_id=%s products=%v poll_interval=%s", c.ID, c.products, c.pollInterval)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.pollLoop()
	}()
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
					c.broadcastStateChange(false)

					c.lastUpdateError = err
					c.backoffErrorCount = c.backoffPolicy.IncError(c.backoffErrorCount)
					log.Errorf("could not update remote-config state:%v; consecutive failures:%d", c.lastUpdateError, consecutiveFailures)
				}
			} else {
				log.Debugf("update successful: successful_first_run:%t, consecutive failures:%d", successfulFirstRun, consecutiveFailures)
				if c.lastUpdateError != nil {
					c.broadcastStateChange(true)
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
//
// Listener dispatch is asynchronous: each listener owns a worker goroutine,
// and update() only enqueues a coalescing wake-up. update() never blocks on
// listener work, so a slow listener cannot delay polling.
func (c *Client) update() error {
	req, err := c.newUpdateRequest()
	if err != nil {
		return err
	}

	// RC-ASYNC: time the fetch so a slow/blocking gRPC call is visible.
	fetchStart := time.Now()
	response, err := c.configFetcher.ClientGetConfigs(c.ctx, req)
	if err != nil {
		log.Infof("RC-ASYNC fetch failed after %s: %v", time.Since(fetchStart), err)
		return err
	}
	log.Infof("RC-ASYNC fetched configs in %s: config_status=%s target_files=%d",
		time.Since(fetchStart), response.ConfigStatus, len(response.TargetFiles))

	// applyUpdate runs WITHOUT c.m: state.Repository synchronizes its own
	// `configs` (configsMu) and `metadata` (metadataMu), so the poll loop's
	// mutation here is already safe against concurrent readers such as the
	// public Client.GetConfigs. Keeping it off c.m means a poll's TUF
	// verification never blocks SubscribeAll / SetAgentName / GetConfigs.
	changedProducts, err := c.applyUpdate(response)
	if err != nil {
		log.Infof("RC-ASYNC applyUpdate error: %v", err)
		return err
	}
	if len(changedProducts) == 0 {
		log.Infof("RC-ASYNC no changed products this poll (nothing to dispatch)")
		return nil
	}
	log.Infof("RC-ASYNC applyUpdate produced changed_products=%v", changedProducts)

	// c.m is taken only for the dispatch below, which reads Client-owned state
	// (c.listeners). It is never held across listener work — scheduleUpdate
	// just stages a payload and signals a worker.
	c.m.Lock()
	defer c.m.Unlock()
	for product, productListeners := range c.listeners {
		if !containsProduct(changedProducts, product) {
			continue
		}
		// Snapshot once per product. state.GetConfigs returns a fresh map
		// (see pkg/remoteconfig/state/configs.go).
		configs := c.state.GetConfigs(product)
		log.Infof("RC-ASYNC dispatching product=%s configs=%d listeners=%d versions=%s",
			product, len(configs), len(productListeners), formatConfigVersions(configs))
		// Bind the apply-status callback to the versions in *this* snapshot.
		// A slow Listener.OnUpdate may finish after the next poll has already
		// replaced the configs at these paths; without version-binding, the
		// late ApplyStatus would stamp the new version's metadata with the
		// result of processing the old config (see
		// state.Repository.UpdateApplyStatusIfVersion). The closure only reads
		// the captured versions and routes through a synchronized repository
		// method, so it is safe to share across this product's listeners.
		applyStatus := c.boundApplyStatus(configs)
		for _, entry := range productListeners {
			if response.ConfigStatus == pbgo.ConfigStatus_CONFIG_STATUS_OK ||
				!entry.listener.ShouldIgnoreSignatureExpiration() {
				// Hand each listener its own map so concurrent workers can't
				// race on a shared instance if a listener mutates its input.
				// This is a shallow clone — it isolates map-level mutations
				// (add/delete/replace), matching the per-listener isolation the
				// synchronous code provided. Config bodies ([]byte) are still
				// shared, exactly as before; they must be treated read-only.
				log.Infof("RC-ASYNC staging update for listener product=%s (%T)", product, entry.listener)
				entry.scheduleUpdate(maps.Clone(configs), applyStatus)
			} else {
				log.Infof("RC-ASYNC skipping listener product=%s (signature expired, listener opts out)", product)
			}
		}
	}
	return nil
}

// formatConfigVersions renders "path=version" pairs for logging. RC-ASYNC debug
// helper only; safe to remove with the RC-ASYNC log lines.
func formatConfigVersions(configs map[string]state.RawConfig) string {
	parts := make([]string, 0, len(configs))
	for path, cfg := range configs {
		parts = append(parts, fmt.Sprintf("%s@v%d", path, cfg.Metadata.Version))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// boundApplyStatus returns an apply-status callback that drops writes for any
// config path whose version no longer matches the one in the supplied
// snapshot. This is the load-bearing piece for correctness against slow
// listeners: a stale callback from an in-progress OnUpdate must not overwrite
// the apply status of a newer config version.
func (c *Client) boundApplyStatus(snapshot map[string]state.RawConfig) func(string, state.ApplyStatus) {
	versions := make(map[string]uint64, len(snapshot))
	for path, cfg := range snapshot {
		versions[path] = cfg.Metadata.Version
	}
	return func(path string, status state.ApplyStatus) {
		expected, ok := versions[path]
		if !ok {
			// Path was not in the snapshot the listener received; nothing to
			// safely write to. Drop.
			log.Infof("RC-ASYNC apply-status DROPPED (unknown path) path=%s state=%d (not in the snapshot this listener received)", path, status.State)
			return
		}
		if c.state.UpdateApplyStatusIfVersion(path, expected, status) {
			log.Infof("RC-ASYNC apply-status APPLIED path=%s v=%d state=%d", path, expected, status.State)
		} else {
			log.Infof("RC-ASYNC apply-status DROPPED (stale version) path=%s expected_v=%d state=%d (superseded by a newer update before the listener acked)", path, expected, status.State)
		}
	}
}

// broadcastStateChange dispatches an OnStateChange signal to every registered
// listener via its worker goroutine. Like update(), this is non-blocking — the
// poll loop is never held up by listener work.
func (c *Client) broadcastStateChange(connected bool) {
	// Best-effort skip post-Close: the workers may have already exited and
	// signaling them just leaves dead wake messages in their channels until GC.
	if c.ctx.Err() != nil {
		return
	}
	c.m.Lock()
	defer c.m.Unlock()
	n := 0
	for _, productListeners := range c.listeners {
		for _, entry := range productListeners {
			entry.scheduleStateChange(connected)
			n++
		}
	}
	log.Infof("RC-ASYNC broadcastStateChange(connected=%t) staged for %d listener(s)", connected, n)
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

// listenerEntry holds a registered Listener and the dispatcher goroutine that
// delivers events to it. One entry per Subscribe / SubscribeAll call.
//
// Dispatch contract:
//   - Calls into the wrapped Listener (OnUpdate, OnStateChange) are always
//     serialized for a given entry — the worker is single-threaded.
//   - Wake signals are coalescing (wake has cap=1). If multiple poll
//     iterations enqueue work before the worker drains, the worker sees only
//     the most-recently-staged payload. This is the property that protects the
//     poll loop from a slow listener: pending iterations collapse instead of
//     piling up.
//   - OnStateChange similarly carries only the latest known state. Rapid
//     toggles can be lost; this matches the existing behaviour of
//     pollLoop, which only fires OnStateChange on transitions.
//   - When the client context is canceled (Client.Close), the worker exits
//     after finishing any in-flight callback. A pending wake that arrives
//     after cancellation is discarded — the worker prefers ctx.Done.
type listenerEntry struct {
	listener Listener
	product  string

	// wake signals "drain pending fields". cap=1 + non-blocking sender =
	// coalescing semantics; queued signals don't grow unbounded.
	wake chan struct{}

	// Fields below are owned by the dispatcher (poll loop side) for writes and
	// the worker for reads/resets. mu protects them.
	//
	// Lock ordering: Client.m must always be acquired BEFORE listenerEntry.mu.
	// The schedule* helpers are called from update()/broadcastStateChange with
	// c.m held; no code path takes the reverse order, and adding one would
	// risk a deadlock. The worker's run loop never acquires c.m, which keeps
	// this discipline easy to maintain.
	mu                 sync.Mutex
	pendingConfigs     map[string]state.RawConfig
	pendingApplyStatus func(string, state.ApplyStatus)
	pendingStateChange *bool
}

// scheduleUpdate stages a new OnUpdate payload and wakes the worker.
// Called from the poll loop with c.m held.
func (e *listenerEntry) scheduleUpdate(configs map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	e.mu.Lock()
	// Overwrite any not-yet-consumed payload: the freshest snapshot supersedes
	// older ones — that's the point of coalescing.
	coalesced := e.pendingConfigs != nil
	e.pendingConfigs = configs
	e.pendingApplyStatus = applyStatus
	e.mu.Unlock()
	if coalesced {
		// The worker hadn't drained the previous payload yet — it's a slow
		// listener and we just collapsed an intermediate update into the latest.
		log.Infof("RC-ASYNC COALESCED update for product=%s (worker still busy; previous snapshot superseded before delivery)", e.product)
	}
	e.signal()
}

// scheduleStateChange stages an OnStateChange and wakes the worker. The latest
// staged value wins.
func (e *listenerEntry) scheduleStateChange(connected bool) {
	v := connected
	e.mu.Lock()
	e.pendingStateChange = &v
	e.mu.Unlock()
	e.signal()
}

func (e *listenerEntry) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
		// A wake is already queued; the worker will pick up the freshest fields
		// when it drains. Dropping here is intentional.
	}
}

// run is the dispatcher loop. Exits when ctx is canceled.
func (e *listenerEntry) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Infof("RC-ASYNC worker exiting product=%s (%T) — context canceled", e.product, e.listener)
			return
		case <-e.wake:
			e.mu.Lock()
			configs := e.pendingConfigs
			applyStatus := e.pendingApplyStatus
			stateChange := e.pendingStateChange
			e.pendingConfigs = nil
			e.pendingApplyStatus = nil
			e.pendingStateChange = nil
			e.mu.Unlock()

			e.deliver(stateChange, configs, applyStatus)
		}
	}
}

// deliver invokes the listener callbacks for a single drain. Wrapped in its
// own function so the deferred recover() runs after each delivery, not at the
// end of run(); a panicking listener stays subscribed and keeps receiving
// future updates, and the process is not torn down by one buggy callback.
//
// OnStateChange runs first: it conveys whether RC is currently reachable,
// which a listener may want to know before processing the accompanying
// OnUpdate (if any).
func (e *listenerEntry) deliver(stateChange *bool, configs map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("remote-config listener for product=%s panicked, dropping this update: %v", e.product, r)
		}
	}()

	if stateChange != nil {
		log.Infof("RC-ASYNC -> OnStateChange(connected=%t) product=%s (%T)", *stateChange, e.product, e.listener)
		e.listener.OnStateChange(*stateChange)
	}
	if configs != nil {
		log.Infof("RC-ASYNC -> OnUpdate product=%s (%T) configs=%d — calling listener", e.product, e.listener, len(configs))
		start := time.Now()
		e.listener.OnUpdate(configs, applyStatus)
		elapsed := time.Since(start)
		// A long elapsed here is the slow-listener case the async dispatch is
		// designed to tolerate — it no longer blocks polling or other listeners.
		log.Infof("RC-ASYNC <- OnUpdate returned product=%s (%T) took=%s", e.product, e.listener, elapsed)
	}
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
