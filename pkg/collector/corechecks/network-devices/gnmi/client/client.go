// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package client maintains streaming gNMI subscriptions and a latest-value cache.
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/admission"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
)

const (
	defaultMinReconnectDelay = 1 * time.Second
	defaultMaxReconnectDelay = 30 * time.Second
)

// Config holds the connection and subscription settings for a gNMI client.
type Config struct {
	Address            string
	Port               int
	Username           string
	Password           string
	Profile            config.ProfileDefinition
	CollectTopology    bool
	UseTLS             bool
	InsecureSkipVerify bool
	Encoding           gnmipb.Encoding
}

// Option configures optional client behavior, primarily for tests.
type Option func(*options)

type options struct {
	minReconnectDelay time.Duration
	maxReconnectDelay time.Duration
	dial              func(context.Context, string, ...grpc.DialOption) (*grpc.ClientConn, error)
}

// WithReconnectDelays overrides reconnect backoff bounds.
func WithReconnectDelays(minDelay, maxDelay time.Duration) Option {
	return func(o *options) {
		o.minReconnectDelay = minDelay
		o.maxReconnectDelay = maxDelay
	}
}

// StreamState describes the current gNMI subscribe stream lifecycle.
type StreamState int

const (
	// StreamStateNotReady means the client has not yet established a subscribe stream.
	StreamStateNotReady StreamState = iota
	// StreamStateConnected means the client is actively receiving updates.
	StreamStateConnected
	// StreamStateReconnecting means the client lost its stream and is reconnecting.
	StreamStateReconnecting
)

// ConnectionStatus exposes connection diagnostics for status reporting.
type ConnectionStatus struct {
	LastError         string
	LastErrorAt       time.Time
	NextReconnectAt   time.Time
	EverConnected     bool
	LastConnectedAt   time.Time
}

// String returns a stable string representation of the stream state.
func (s StreamState) String() string {
	switch s {
	case StreamStateNotReady:
		return "not_ready"
	case StreamStateConnected:
		return "connected"
	case StreamStateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// Client maintains a streaming gNMI subscription and a latest-value cache.
type Client struct {
	cfg Config
	opt options

	transport TransportConfig

	mu       sync.Mutex
	conn     *grpc.ClientConn
	cancel   context.CancelFunc
	done     chan struct{}
	closeErr error

	cache *cache

	synchronized atomic.Bool

	reconnectAttempts int
	streamState       StreamState
	everConnected     bool
	lastConnectedAt   time.Time

	lastStreamError   string
	lastStreamErrorAt time.Time
	nextReconnectAt   time.Time
}

// New creates a client. Call Start to open the subscription loop.
func New(cfg Config, opts ...Option) (*Client, error) {
	if cfg.Address == "" {
		return nil, errors.New("address is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d", cfg.Port)
	}
	if cfg.Username == "" {
		return nil, errors.New("username is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("password is required")
	}
	if len(cfg.Profile.Metrics) == 0 {
		return nil, errors.New("profile must define at least one metric")
	}
	if cfg.Encoding == 0 {
		cfg.Encoding = config.DefaultEncoding
	}

	clientOpts := options{
		minReconnectDelay: defaultMinReconnectDelay,
		maxReconnectDelay: defaultMaxReconnectDelay,
		dial: func(_ context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return grpc.NewClient(target, opts...)
		},
	}
	for _, opt := range opts {
		opt(&clientOpts)
	}
	if clientOpts.minReconnectDelay <= 0 {
		return nil, errors.New("min reconnect delay must be greater than 0")
	}
	if clientOpts.maxReconnectDelay < clientOpts.minReconnectDelay {
		return nil, fmt.Errorf("max reconnect delay %s must be >= min reconnect delay %s", clientOpts.maxReconnectDelay, clientOpts.minReconnectDelay)
	}

	return &Client{
		cfg:   cfg,
		opt:   clientOpts,
		transport: TransportConfig{
			UseTLS:             cfg.UseTLS,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		},
		cache: newCache(),
	}, nil
}

// Start launches the reconnect loop and receive goroutine.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		return errors.New("client already started")
	}

	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})

	go c.run(runCtx)
	return nil
}

// Close stops the reconnect loop, closes the active stream and connection, and waits for shutdown.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.cancel == nil {
		c.mu.Unlock()
		return nil
	}
	cancel := c.cancel
	done := c.done
	c.cancel = nil
	c.mu.Unlock()

	cancel()
	<-done

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeErr != nil && errors.Is(c.closeErr, context.Canceled) {
		return nil
	}
	return c.closeErr
}

// Cancel is an alias for Close.
func (c *Client) Cancel() error {
	return c.Close()
}

// Get returns the latest cached value for a normalized path and key set.
func (c *Client) Get(path string, keys map[string]string) (CacheEntry, bool) {
	return c.cache.get(CacheKey{Path: path, Keys: cloneKeys(keys)})
}

// CachedValue pairs a cache key with its latest entry.
type CachedValue struct {
	Key   CacheKey
	Entry CacheEntry
}

// Snapshot returns a copy of all cached entries.
func (c *Client) Snapshot() []CachedValue {
	return c.cache.snapshot()
}

// ReconnectAttempts returns the number of reconnect attempts since the last successful stream.
func (c *Client) ReconnectAttempts() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reconnectAttempts
}

// StreamState returns the current subscribe stream lifecycle state.
func (c *Client) StreamState() StreamState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streamState
}

// Synchronized reports whether the active subscribe stream has received sync_response.
func (c *Client) Synchronized() bool {
	return c.synchronized.Load()
}

// ReceivedSamples returns the number of cached samples.
func (c *Client) ReceivedSamples() int {
	return c.cache.count()
}

// TransportMode returns the configured transport security mode.
func (c *Client) TransportMode() TransportMode {
	return c.transport.mode()
}

// ConnectionStatus returns the latest connection error and reconnect timing.
func (c *Client) ConnectionStatus() ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	return ConnectionStatus{
		LastError:       c.lastStreamError,
		LastErrorAt:     c.lastStreamErrorAt,
		NextReconnectAt: c.nextReconnectAt,
		EverConnected:   c.everConnected,
		LastConnectedAt: c.lastConnectedAt,
	}
}

func (c *Client) setStreamState(state StreamState) {
	c.mu.Lock()
	c.streamState = state
	c.mu.Unlock()
}

func (c *Client) setPreConnectStreamState() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.everConnected {
		c.streamState = StreamStateReconnecting
	} else {
		c.streamState = StreamStateNotReady
	}
}

func (c *Client) setPostDisconnectStreamState() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.everConnected {
		c.streamState = StreamStateReconnecting
	} else {
		c.streamState = StreamStateNotReady
	}
}

func (c *Client) run(ctx context.Context) {
	defer close(c.done)

	c.setStreamState(StreamStateNotReady)

	policy := backoff.NewExpBackoffPolicy(
		2,
		float64(c.opt.minReconnectDelay)/float64(time.Second),
		float64(c.opt.maxReconnectDelay)/float64(time.Second),
		1,
		false,
	)

	numErrors := 0
	for {
		if ctx.Err() != nil {
			c.setCloseErr(ctx.Err())
			return
		}

		c.setPreConnectStreamState()

		err := c.connectAndReceive(ctx)
		if ctx.Err() != nil {
			c.setCloseErr(ctx.Err())
			return
		}
		if err != nil {
			c.setPostDisconnectStreamState()
			numErrors = policy.IncError(numErrors)
			delay := policy.GetBackoffDuration(numErrors)
			c.recordReconnectError(err, numErrors, delay)

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				c.setCloseErr(ctx.Err())
				return
			case <-timer.C:
			}
		}
	}
}

func (c *Client) connectAndReceive(ctx context.Context) error {
	c.cache.clear()
	c.synchronized.Store(false)

	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.reconnectAttempts = 0
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		admission.Gate().Release()
		_ = conn.Close()
	}()

	gnmiClient := gnmipb.NewGNMIClient(conn)
	stream, err := gnmiClient.Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("open subscribe stream: %w", err)
	}

	subscribeReq, err := c.buildSubscribeRequest()
	if err != nil {
		return err
	}
	if err := stream.Send(subscribeReq); err != nil {
		return fmt.Errorf("send subscribe request: %w", err)
	}

	now := time.Now()
	c.mu.Lock()
	c.everConnected = true
	c.lastConnectedAt = now
	c.streamState = StreamStateConnected
	c.lastStreamError = ""
	c.lastStreamErrorAt = time.Time{}
	c.nextReconnectAt = time.Time{}
	c.mu.Unlock()

	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return errors.New("subscribe stream closed")
			}
			return fmt.Errorf("receive subscribe response: %w", err)
		}

		if err := c.handleSubscribeResponse(resp); err != nil {
			return err
		}
	}
}

func (c *Client) dial(ctx context.Context) (*grpc.ClientConn, error) {
	if err := admission.Gate().Admit(ctx); err != nil {
		return nil, fmt.Errorf("admission gate: %w", err)
	}

	target := net.JoinHostPort(c.cfg.Address, strconv.Itoa(c.cfg.Port))
	secure := c.transport.UseTLS
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(c.transport.transportCredentials()),
		grpc.WithPerRPCCredentials(newPassCred(c.cfg.Username, c.cfg.Password, secure)),
	}
	conn, err := c.opt.dial(ctx, target, dialOpts...)
	if err != nil {
		admission.Gate().Release()
		return nil, fmt.Errorf("dial %s: %w", target, err)
	}
	return conn, nil
}

func (c *Client) buildSubscribeRequest() (*gnmipb.SubscribeRequest, error) {
	specs := buildSubscriptionSpecs(c.cfg)
	subscriptions := make([]*gnmipb.Subscription, 0, len(specs))
	for _, spec := range specs {
		path, err := subscribePathFromSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("path %q: %w", spec.Path, err)
		}
		subscriptions = append(subscriptions, &gnmipb.Subscription{
			Path: path,
			Mode: gnmipb.SubscriptionMode_SAMPLE,
		})
	}

	return &gnmipb.SubscribeRequest{
		Request: &gnmipb.SubscribeRequest_Subscribe{
			Subscribe: &gnmipb.SubscriptionList{
				Mode:         gnmipb.SubscriptionList_STREAM,
				Encoding:     c.cfg.Encoding,
				Subscription: subscriptions,
			},
		},
	}, nil
}

func (c *Client) handleSubscribeResponse(resp *gnmipb.SubscribeResponse) error {
	switch payload := resp.GetResponse().(type) {
	case *gnmipb.SubscribeResponse_SyncResponse:
		c.synchronized.Store(true)
		return nil
	case *gnmipb.SubscribeResponse_Update:
		c.applyNotification(payload.Update)
		return nil
	default:
		return fmt.Errorf("unsupported subscribe response type %T", payload)
	}
}

func (c *Client) applyNotification(notification *gnmipb.Notification) {
	if notification == nil {
		return
	}

	timestamp := time.Unix(0, notification.GetTimestamp())
	for _, update := range notification.GetUpdate() {
		c.applyUpdate(update, timestamp)
	}
	for _, deletedPath := range notification.GetDelete() {
		c.applyDelete(deletedPath)
	}
}

func (c *Client) applyUpdate(update *gnmipb.Update, timestamp time.Time) {
	if update == nil || update.GetPath() == nil {
		return
	}

	value, err := decodeTypedValue(update.GetVal())
	if err != nil {
		return
	}

	key := cacheKeyFromGNMIPath(update.GetPath())
	entry := CacheEntry{
		Value:     value,
		Timestamp: timestamp,
		Keys:      cloneKeys(key.Keys),
	}
	c.cache.set(key, entry)

	for _, leaf := range flattenJSONLeaves(key.Path, key.Keys, value, timestamp) {
		c.cache.set(leaf.Key, leaf.Entry)
	}
}

func (c *Client) applyDelete(path *gnmipb.Path) {
	key := cacheKeyFromGNMIPath(path)
	if len(key.Keys) == 0 {
		c.cache.deletePrefix(key)
		return
	}
	c.cache.delete(key)
}

func (c *Client) setCloseErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeErr == nil {
		c.closeErr = err
	}
}

func (c *Client) recordReconnectError(err error, numErrors int, delay time.Duration) {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.reconnectAttempts = numErrors
	if err != nil {
		c.lastStreamError = err.Error()
		c.lastStreamErrorAt = now
	}
	if delay > 0 {
		c.nextReconnectAt = now.Add(delay)
	} else {
		c.nextReconnectAt = time.Time{}
	}
}
