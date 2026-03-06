// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumerimpl implements the configstreamconsumer component
package configstreamconsumerimpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// Requires defines the dependencies for the configstreamconsumer component
type Requires struct {
	compdef.In

	Lifecycle compdef.Lifecycle
	Log       log.Component
	IPC       ipc.Component
	Telemetry telemetry.Component
	// Params is optional; when nil the component is not created (e.g. when RAR is disabled).
	Params *Params `optional:"true"`
}

// SessionIDProvider supplies the RAR session ID, typically after registration completes.
// When set, the consumer will call WaitSessionID at connect time instead of using Params.SessionID.
type SessionIDProvider interface {
	WaitSessionID(ctx context.Context) (string, error)
}

// Params defines the parameters for the configstreamconsumer component
type Params struct {
	// ClientName is the identity of this remote agent (e.g., "system-probe", "trace-agent")
	ClientName string
	// CoreAgentAddress is the address of the core agent IPC endpoint
	CoreAgentAddress string
	// SessionID is the RAR session ID for authorization. Required if SessionIDProvider is nil.
	SessionID string
	// SessionIDProvider supplies the session ID at connect time (e.g. from remote agent component).
	// When set, SessionID may be empty; the consumer will block on WaitSessionID before connecting.
	SessionIDProvider SessionIDProvider
	// ConfigWriter if set receives streamed config updates (same source as configsync: SourceLocalConfigProcess).
	// Used by remote agents to mirror core agent config into the local config.Component.
	ConfigWriter model.Writer
}

// Provides defines the output of the configstreamconsumer component
type Provides struct {
	compdef.Out

	Comp configstreamconsumer.Component
}

// consumer implements the configstreamconsumer.Component interface
type consumer struct {
	log       log.Component
	ipc       ipc.Component
	telemetry telemetry.Component
	params    Params

	conn       *grpc.ClientConn
	client     pb.AgentSecureClient
	stream     pb.AgentSecure_StreamConfigEventsClient
	streamLock sync.Mutex

	effectiveConfig map[string]interface{}
	configLock      sync.RWMutex
	lastSeqID       int32
	reader          *configReader

	ready     bool
	readyCh   chan struct{}
	readyOnce sync.Once

	subscribers   []chan configstreamconsumer.ChangeEvent
	subscribersMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	metricsInitOnce         sync.Once
	timeToFirstSnapshot     telemetry.Gauge
	streamReconnectCount    telemetry.Counter
	lastSeqIDMetric         telemetry.Gauge
	droppedStaleUpdates     telemetry.Counter
	bufferOverflowDiscounts telemetry.Counter
}

// NewComponent creates a new configstreamconsumer component
func NewComponent(reqs Requires) (Provides, error) {
	if reqs.Params == nil {
		return Provides{}, nil
	}
	p := *reqs.Params
	if p.ClientName == "" {
		return Provides{}, errors.New("ClientName is required")
	}
	if p.CoreAgentAddress == "" {
		return Provides{}, errors.New("CoreAgentAddress is required")
	}
	// When both are empty the component is disabled (e.g. RAR not enabled).
	if p.SessionID == "" && p.SessionIDProvider == nil {
		return Provides{}, nil
	}
	hasID := p.SessionID != ""
	hasProvider := p.SessionIDProvider != nil
	if hasID == hasProvider {
		return Provides{}, errors.New("exactly one of SessionID or SessionIDProvider must be set")
	}

	c := &consumer{
		log:             reqs.Log,
		ipc:             reqs.IPC,
		telemetry:       reqs.Telemetry,
		params:          p,
		effectiveConfig: make(map[string]interface{}),
		readyCh:         make(chan struct{}),
	}

	c.reader = &configReader{consumer: c}

	// Register lifecycle hooks
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			return c.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			c.stop()
			return nil
		},
	})

	return Provides{Comp: c}, nil
}

// Start initiates the config stream connection and processing loop
func (c *consumer) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.initMetrics()

	// Start the stream loop in a goroutine
	c.wg.Add(1)
	go c.streamLoop()

	return nil
}

// stop gracefully shuts down the consumer
func (c *consumer) stop() {
	c.cancel()
	c.streamLock.Lock()
	if c.stream != nil {
		_ = c.stream.CloseSend()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.streamLock.Unlock()
	c.wg.Wait()
}

// WaitReady blocks until the first config snapshot has been received and applied
func (c *consumer) WaitReady(ctx context.Context) error {
	select {
	case <-c.readyCh:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for config snapshot: %w", ctx.Err())
	}
}

// Reader returns a config reader backed by the streamed configuration
func (c *consumer) Reader() model.Reader {
	return c.reader
}

// Subscribe returns a channel that receives config change events
func (c *consumer) Subscribe() (<-chan configstreamconsumer.ChangeEvent, func()) {
	ch := make(chan configstreamconsumer.ChangeEvent, 100)

	c.subscribersMu.Lock()
	c.subscribers = append(c.subscribers, ch)
	idx := len(c.subscribers) - 1
	c.subscribersMu.Unlock()

	unsubscribe := func() {
		c.subscribersMu.Lock()
		defer c.subscribersMu.Unlock()
		if idx < len(c.subscribers) {
			close(c.subscribers[idx])
			c.subscribers[idx] = nil
		}
	}

	return ch, unsubscribe
}

// streamLoop manages the lifecycle of the config stream connection
func (c *consumer) streamLoop() {
	defer c.wg.Done()

	startTime := time.Now()
	firstSnapshot := true

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Establish connection and stream
		if err := c.connectAndStream(startTime, &firstSnapshot); err != nil {
			if err == context.Canceled || c.ctx.Err() != nil {
				return
			}
			c.log.Warnf("Config stream error: %v, reconnecting...", err)
			c.streamReconnectCount.Inc()

			// Exponential backoff for reconnection
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}
	}
}

// connectAndStream establishes a gRPC connection and processes the config stream
func (c *consumer) connectAndStream(startTime time.Time, firstSnapshot *bool) error {
	conn, err := grpc.NewClient(c.params.CoreAgentAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(c.ipc.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(c.ipc.GetAuthToken())),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to core agent: %w", err)
	}

	c.streamLock.Lock()
	c.conn = conn
	c.client = pb.NewAgentSecureClient(conn)
	c.streamLock.Unlock()

	sessionID := c.params.SessionID
	if c.params.SessionIDProvider != nil {
		var err error
		sessionID, err = c.params.SessionIDProvider.WaitSessionID(c.ctx)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("waiting for session ID: %w", err)
		}
	}
	// Add session_id to gRPC metadata
	md := metadata.New(map[string]string{"session_id": sessionID})
	ctxWithMetadata := metadata.NewOutgoingContext(c.ctx, md)

	// Start streaming
	stream, err := c.client.StreamConfigEvents(ctxWithMetadata, &pb.ConfigStreamRequest{
		Name: c.params.ClientName,
	})
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to start config stream: %w", err)
	}

	c.streamLock.Lock()
	c.stream = stream
	c.streamLock.Unlock()

	c.log.Infof("Config stream established for client %s", c.params.ClientName)

	// Process stream events
	for {
		event, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				c.log.Info("Config stream closed by server")
				return nil
			}
			return fmt.Errorf("stream receive error: %w", err)
		}

		if err := c.handleConfigEvent(event, startTime, firstSnapshot); err != nil {
			c.log.Errorf("Failed to handle config event: %v", err)
		}
	}
}

// handleConfigEvent processes a single config event from the stream
func (c *consumer) handleConfigEvent(event *pb.ConfigEvent, startTime time.Time, firstSnapshot *bool) error {
	switch e := event.Event.(type) {
	case *pb.ConfigEvent_Snapshot:
		return c.applySnapshot(e.Snapshot, startTime, firstSnapshot)
	case *pb.ConfigEvent_Update:
		return c.applyUpdate(e.Update)
	default:
		return fmt.Errorf("unknown event type: %T", event.Event)
	}
}

// applySnapshot applies a complete config snapshot
func (c *consumer) applySnapshot(snapshot *pb.ConfigSnapshot, startTime time.Time, firstSnapshot *bool) error {
	// Check for stale snapshot
	if snapshot.SequenceId <= c.lastSeqID {
		c.log.Debugf("Ignoring stale snapshot (seq_id: %d <= %d)", snapshot.SequenceId, c.lastSeqID)
		c.droppedStaleUpdates.Inc()
		return nil
	}

	c.log.Infof("Applying config snapshot (seq_id: %d, settings: %d)", snapshot.SequenceId, len(snapshot.Settings))

	// Convert protobuf settings to Go map
	newConfig := make(map[string]interface{}, len(snapshot.Settings))
	for _, setting := range snapshot.Settings {
		newConfig[setting.Key] = pbValueToGo(setting.Value)
	}

	// Update effective config atomically
	c.configLock.Lock()
	oldConfig := c.effectiveConfig
	c.effectiveConfig = newConfig
	c.lastSeqID = snapshot.SequenceId
	c.configLock.Unlock()

	c.lastSeqIDMetric.Set(float64(snapshot.SequenceId))

	// Mark as ready if this is the first snapshot
	if *firstSnapshot {
		*firstSnapshot = false
		c.readyOnce.Do(func() {
			close(c.readyCh)
			c.ready = true
			duration := time.Since(startTime)
			c.timeToFirstSnapshot.Set(duration.Seconds())
			c.log.Infof("Received first config snapshot after %v", duration)
		})
	}

	c.emitChangeEvents(oldConfig, newConfig)

	if c.params.ConfigWriter != nil {
		for key, val := range newConfig {
			c.params.ConfigWriter.Set(key, val, model.SourceLocalConfigProcess)
		}
	}

	return nil
}

// applyUpdate applies a single config update
func (c *consumer) applyUpdate(update *pb.ConfigUpdate) error {
	// Check for stale update
	if update.SequenceId <= c.lastSeqID {
		c.log.Debugf("Ignoring stale update (seq_id: %d <= %d)", update.SequenceId, c.lastSeqID)
		c.droppedStaleUpdates.Inc()
		return nil
	}

	// Detect discontinuity
	if update.SequenceId != c.lastSeqID+1 {
		c.log.Warnf("Discontinuity detected: expected seq_id %d, got %d", c.lastSeqID+1, update.SequenceId)
		// The server should automatically send a snapshot to resync
		return nil
	}

	c.log.Debugf("Applying config update (seq_id: %d, key: %s)", update.SequenceId, update.Setting.Key)

	c.configLock.Lock()
	oldValue := c.effectiveConfig[update.Setting.Key]
	newValue := pbValueToGo(update.Setting.Value)
	c.effectiveConfig[update.Setting.Key] = newValue
	c.lastSeqID = update.SequenceId
	c.configLock.Unlock()

	c.lastSeqIDMetric.Set(float64(update.SequenceId))

	c.emitChangeEvent(configstreamconsumer.ChangeEvent{
		Key:      update.Setting.Key,
		OldValue: oldValue,
		NewValue: newValue,
	})

	if c.params.ConfigWriter != nil {
		c.params.ConfigWriter.Set(update.Setting.Key, newValue, model.SourceLocalConfigProcess)
	}

	return nil
}

// emitChangeEvents emits change events for all differences between old and new config
func (c *consumer) emitChangeEvents(oldConfig, newConfig map[string]interface{}) {
	// Find changed and added keys
	for key, newVal := range newConfig {
		oldVal, existed := oldConfig[key]
		if !existed || !valuesEqual(oldVal, newVal) {
			c.emitChangeEvent(configstreamconsumer.ChangeEvent{
				Key:      key,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	// Find deleted keys
	for key, oldVal := range oldConfig {
		if _, exists := newConfig[key]; !exists {
			c.emitChangeEvent(configstreamconsumer.ChangeEvent{
				Key:      key,
				OldValue: oldVal,
				NewValue: nil,
			})
		}
	}
}

// emitChangeEvent sends a change event to all subscribers
func (c *consumer) emitChangeEvent(event configstreamconsumer.ChangeEvent) {
	c.subscribersMu.RLock()
	defer c.subscribersMu.RUnlock()

	for _, ch := range c.subscribers {
		if ch == nil {
			continue
		}
		select {
		case ch <- event:
		default:
			c.log.Warnf("Subscriber buffer full, dropping event for key: %s", event.Key)
		}
	}
}

// initMetrics initializes telemetry metrics
func (c *consumer) initMetrics() {
	c.metricsInitOnce.Do(func() {
		c.timeToFirstSnapshot = c.telemetry.NewGauge(
			"configstream_consumer",
			"time_to_first_snapshot_seconds",
			[]string{},
			"Time taken to receive the first config snapshot",
		)
		c.streamReconnectCount = c.telemetry.NewCounter(
			"configstream_consumer",
			"reconnect_count",
			[]string{},
			"Number of times the config stream has reconnected",
		)
		c.lastSeqIDMetric = c.telemetry.NewGauge(
			"configstream_consumer",
			"last_sequence_id",
			[]string{},
			"Last received config sequence ID",
		)
		c.droppedStaleUpdates = c.telemetry.NewCounter(
			"configstream_consumer",
			"dropped_stale_updates",
			[]string{},
			"Number of stale config updates dropped",
		)
		c.bufferOverflowDiscounts = c.telemetry.NewCounter(
			"configstream_consumer",
			"buffer_overflow_disconnects",
			[]string{},
			"Number of stream disconnects due to buffer overflow",
		)
	})
}

// pbValueToGo converts a protobuf Value to a Go value
func pbValueToGo(pbValue *structpb.Value) interface{} {
	if pbValue == nil {
		return nil
	}

	// AsInterface() converts all numbers to float64 (JSON semantics).
	// Try to preserve integer types when the float has no fractional part.
	result := pbValue.AsInterface()

	if f, ok := result.(float64); ok {
		// Check if this float represents an integer value
		// Only convert if within the safe integer range for float64
		const maxSafeInteger = 1 << 53 // 2^53
		const minSafeInteger = -maxSafeInteger

		if f >= minSafeInteger && f <= maxSafeInteger && f == float64(int64(f)) {
			// No fractional part and within safe range - convert to int64
			return int64(f)
		}
	}

	return result
}

// valuesEqual checks if two values are equal (simplified comparison)
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
