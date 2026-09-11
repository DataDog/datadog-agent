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
	"time"

	"github.com/benbjohnson/clock"
	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	parentgnmi "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
)

const (
	defaultMinReconnectDelay = 1 * time.Second
	defaultMaxReconnectDelay = 30 * time.Second
)

// Config holds the connection and subscription settings for a gNMI client.
type Config struct {
	Address  string
	Port     int
	Username string
	Password string
	Profile  config.ProfileDefinition
}

// Option configures optional client behavior, primarily for tests.
type Option func(*options)

type options struct {
	minReconnectDelay time.Duration
	maxReconnectDelay time.Duration
	dial              func(context.Context, string, ...grpc.DialOption) (*grpc.ClientConn, error)
	clock             clock.Clock
}

// WithReconnectDelays overrides reconnect backoff bounds.
func WithReconnectDelays(minDelay, maxDelay time.Duration) Option {
	return func(o *options) {
		o.minReconnectDelay = minDelay
		o.maxReconnectDelay = maxDelay
	}
}

// WithClock overrides the clock used by the reconnect loop.
func WithClock(clk clock.Clock) Option {
	return func(o *options) {
		o.clock = clk
	}
}

// Client maintains a streaming gNMI subscription and a latest-value cache.
type Client struct {
	cfg Config
	opt options

	mu       sync.Mutex
	started  bool
	conn     *grpc.ClientConn
	cancel   context.CancelFunc
	done     chan struct{}
	closeErr error

	cache *cache

	reconnectAttempts int
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

	clientOpts := options{
		minReconnectDelay: defaultMinReconnectDelay,
		maxReconnectDelay: defaultMaxReconnectDelay,
		dial: func(_ context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
			return grpc.NewClient(target, opts...)
		},
		clock: clock.New(),
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
	if clientOpts.clock == nil {
		return nil, errors.New("clock is required")
	}

	return &Client{
		cfg:   cfg,
		opt:   clientOpts,
		cache: newCache(),
	}, nil
}

// Start launches the reconnect loop and receive goroutine.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return errors.New("client already started")
	}

	runCtx, cancel := context.WithCancel(ctx)
	c.started = true
	c.cancel = cancel
	c.done = make(chan struct{})
	c.closeErr = nil
	c.reconnectAttempts = 0

	go c.run(runCtx)
	return nil
}

// Close stops the reconnect loop, closes the active stream and connection, and waits for shutdown.
func (c *Client) Close() error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return nil
	}
	cancel := c.cancel
	done := c.done
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

func (c *Client) run(ctx context.Context) {
	defer close(c.done)

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

		synced, err := c.connectAndReceive(ctx)
		if ctx.Err() != nil {
			c.setCloseErr(ctx.Err())
			return
		}
		if err != nil {
			if synced {
				numErrors = 0
			}
			numErrors = policy.IncError(numErrors)
			c.setReconnectAttempts(numErrors)

			delay := policy.GetBackoffDuration(numErrors)
			timer := c.opt.clock.Timer(delay)
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

func (c *Client) connectAndReceive(ctx context.Context) (bool, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return false, err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		_ = conn.Close()
	}()

	gnmiClient := gnmipb.NewGNMIClient(conn)
	streamCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"username", c.cfg.Username,
		"password", c.cfg.Password,
	))
	stream, err := gnmiClient.Subscribe(streamCtx)
	if err != nil {
		return false, fmt.Errorf("open subscribe stream: %w", err)
	}

	subscribeReq, err := c.buildSubscribeRequest()
	if err != nil {
		return false, err
	}
	if err := stream.Send(subscribeReq); err != nil {
		return false, fmt.Errorf("send subscribe request: %w", err)
	}

	initialCache := newCache()
	synced := false
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return synced, ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return synced, errors.New("subscribe stream closed")
			}
			return synced, fmt.Errorf("receive subscribe response: %w", err)
		}

		targetCache := c.cache
		if !synced {
			targetCache = initialCache
		}
		isSync, err := c.handleSubscribeResponse(targetCache, resp)
		if err != nil {
			return synced, err
		}
		if isSync && !synced {
			c.cache.replace(initialCache)
			c.setReconnectAttempts(0)
			synced = true
		}
	}
}

func (c *Client) dial(ctx context.Context) (*grpc.ClientConn, error) {
	target := net.JoinHostPort(c.cfg.Address, strconv.Itoa(c.cfg.Port))
	// MVP uses insecure transport credentials; production TLS support is follow-up work.
	conn, err := c.opt.dial(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", target, err)
	}
	return conn, nil
}

func (c *Client) buildSubscribeRequest() (*gnmipb.SubscribeRequest, error) {
	subscriptions := make([]*gnmipb.Subscription, 0, len(c.cfg.Profile.Metrics))
	for _, metric := range c.cfg.Profile.Metrics {
		path, err := subscribePathFromMetric(metric)
		if err != nil {
			return nil, fmt.Errorf("metric %q: %w", metric.Metric, err)
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
				Encoding:     parentgnmi.DefaultEncoding,
				Subscription: subscriptions,
			},
		},
	}, nil
}

func (c *Client) handleSubscribeResponse(targetCache *cache, resp *gnmipb.SubscribeResponse) (bool, error) {
	switch payload := resp.GetResponse().(type) {
	case *gnmipb.SubscribeResponse_SyncResponse:
		if !payload.SyncResponse {
			return false, errors.New("received false sync response")
		}
		return true, nil
	case *gnmipb.SubscribeResponse_Update:
		c.applyNotification(targetCache, payload.Update)
		return false, nil
	default:
		return false, fmt.Errorf("unsupported subscribe response type %T", payload)
	}
}

func (c *Client) applyNotification(targetCache *cache, notification *gnmipb.Notification) {
	if notification == nil {
		return
	}

	timestamp := time.Unix(0, notification.GetTimestamp())
	for _, update := range notification.GetUpdate() {
		c.applyUpdate(targetCache, notification.GetPrefix(), update, timestamp)
	}
	for _, deletedPath := range notification.GetDelete() {
		c.applyDelete(targetCache, notification.GetPrefix(), deletedPath)
	}
}

func (c *Client) applyUpdate(targetCache *cache, prefix *gnmipb.Path, update *gnmipb.Update, timestamp time.Time) {
	if update == nil || update.GetPath() == nil {
		return
	}

	value, err := decodeTypedValue(update.GetVal())
	if err != nil {
		return
	}

	path := normalizedPathFromGNMIPath(joinGNMIPaths(prefix, update.GetPath()))
	key := path.cacheKey()
	targetCache.set(path, CacheEntry{
		Value:     value,
		Timestamp: timestamp,
		Keys:      cloneKeys(key.Keys),
	})
}

func (c *Client) applyDelete(targetCache *cache, prefix, path *gnmipb.Path) {
	if path == nil {
		return
	}
	targetCache.deletePrefix(normalizedPathFromGNMIPath(joinGNMIPaths(prefix, path)))
}

func (c *Client) setReconnectAttempts(attempts int) {
	c.mu.Lock()
	c.reconnectAttempts = attempts
	c.mu.Unlock()
}

func (c *Client) setCloseErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeErr == nil {
		c.closeErr = err
	}
}
