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

	Lifecycle    compdef.Lifecycle
	Log          log.Component
	IPC          ipc.Component
	Telemetry    telemetry.Component
	ConfigWriter model.Writer
	Params       Params
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
	// ReadyTimeout is how long OnStart blocks waiting for the first config snapshot before
	// returning an error and aborting startup. Defaults to 60s when zero.
	ReadyTimeout time.Duration
}

// Provides defines the output of the configstreamconsumer component
type Provides struct {
	compdef.Out

	Comp configstreamconsumer.Component
}

// consumer implements the configstreamconsumer.Component interface
type consumer struct {
	log          log.Component
	ipc          ipc.Component
	telemetry    telemetry.Component
	params       Params
	configWriter model.Writer // writes streamed config into the local config.Component

	conn       *grpc.ClientConn
	client     pb.AgentSecureClient
	stream     pb.AgentSecure_StreamConfigEventsClient
	streamLock sync.Mutex

	effectiveConfig map[string]interface{}
	configLock      sync.RWMutex
	lastSeqID       int32

	ready     bool
	readyCh   chan struct{}
	readyOnce sync.Once

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	metricsInitOnce      sync.Once
	timeToFirstSnapshot  telemetry.Gauge
	streamReconnectCount telemetry.Counter
	lastSeqIDMetric      telemetry.Gauge
	droppedStaleUpdates  telemetry.Counter
}

// NewComponent creates a new configstreamconsumer component
func NewComponent(reqs Requires) (Provides, error) {
	p := reqs.Params
	if p.ClientName == "" {
		return Provides{}, errors.New("ClientName is required")
	}
	if p.CoreAgentAddress == "" {
		return Provides{}, errors.New("CoreAgentAddress is required")
	}
	if p.SessionID == "" && p.SessionIDProvider == nil {
		reqs.Log.Errorf("configstreamconsumer: neither SessionID nor SessionIDProvider set for client %s; component will not connect", p.ClientName)
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
		configWriter:    reqs.ConfigWriter,
		effectiveConfig: make(map[string]interface{}),
		readyCh:         make(chan struct{}),
	}

	// Register lifecycle hooks
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: c.Start,
		OnStop:  c.stop,
	})

	return Provides{Comp: c}, nil
}

// Start initiates the config stream connection and blocks until the first config snapshot is
// received. Blocking here ensures all components initialized after this one (and the binary's
// run function) see a fully-populated config. Returns an error if the snapshot is not received
// within ReadyTimeout (default 60s), which aborts FX startup.
func (c *consumer) Start(_ context.Context) error {
	// Use context.Background() so the stream lifetime is not bounded by the
	// Fx startup context, which expires after app.StartTimeout (~5 minutes).
	c.ctx, c.cancel = context.WithCancel(context.Background())

	c.initMetrics()

	c.wg.Add(1)
	go c.streamLoop()

	timeout := c.params.ReadyTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c.log.Infof("Waiting for initial configuration from core agent (timeout: %v)...", timeout)
	if err := c.WaitReady(ctx); err != nil {
		c.cancel()
		c.wg.Wait()
		return fmt.Errorf("waiting for initial config snapshot: %w", err)
	}
	c.log.Infof("Initial configuration received from core agent.")
	return nil
}

// stop gracefully shuts down the consumer
func (c *consumer) stop(_ context.Context) error {
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
	return nil
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
	// Ensure conn is closed when this invocation exits (EOF, error, or context cancel).
	defer conn.Close()

	c.streamLock.Lock()
	c.conn = conn
	c.client = pb.NewAgentSecureClient(conn)
	c.streamLock.Unlock()

	sessionID := c.params.SessionID
	if c.params.SessionIDProvider != nil {
		var err error
		sessionID, err = c.params.SessionIDProvider.WaitSessionID(c.ctx)
		if err != nil {
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
	// Reject out-of-order or server-restart snapshots. lastSeqID is never reset between
	// reconnects: if the server restarts and its sequence counter resets to a lower value,
	// we refuse the new snapshot and log an error. Sub-processes are expected to restart
	// when the core agent restarts.
	if snapshot.SequenceId <= c.lastSeqID {
		c.log.Errorf("Received snapshot with seq_id %d <= current %d; the core agent may have restarted. "+
			"This sub-process must be restarted to accept a new configuration.", snapshot.SequenceId, c.lastSeqID)
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
	c.effectiveConfig = newConfig
	c.lastSeqID = snapshot.SequenceId
	c.configLock.Unlock()

	c.lastSeqIDMetric.Set(float64(snapshot.SequenceId))

	if c.configWriter != nil {
		for _, setting := range snapshot.Settings {
			c.configWriter.Set(setting.Key, newConfig[setting.Key], model.Source(setting.Source))
		}
	}

	// Signal readiness only after the snapshot is fully applied and config mirrored.
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

	return nil
}

// applyUpdate applies a single config update
func (c *consumer) applyUpdate(update *pb.ConfigUpdate) error {
	// Check for stale update
	if update.SequenceId <= c.lastSeqID {
		c.log.Warnf("Ignoring stale update (seq_id: %d <= %d)", update.SequenceId, c.lastSeqID)
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

	newValue := pbValueToGo(update.Setting.Value)

	c.configLock.Lock()
	c.effectiveConfig[update.Setting.Key] = newValue
	c.lastSeqID = update.SequenceId
	c.configLock.Unlock()

	c.lastSeqIDMetric.Set(float64(update.SequenceId))

	if c.configWriter != nil {
		c.configWriter.Set(update.Setting.Key, newValue, model.Source(update.Setting.Source))
	}

	return nil
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
