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
	"math"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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
	Params       configstreamconsumer.Params
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
	params       configstreamconsumer.Params
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

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startTime time.Time

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
		return Provides{}, fmt.Errorf("configstreamconsumer: neither SessionID nor SessionIDProvider set for client %s", p.ClientName)
	}
	if p.SessionID != "" && p.SessionIDProvider != nil {
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

	c.initMetrics()

	// Register lifecycle hooks
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})

	return Provides{Comp: c}, nil
}

// start initiates the config stream connection and blocks until the first config snapshot is
// received. Blocking here ensures all components initialized after this one (and the binary's
// run function) see a fully-populated config. Returns an error if the snapshot is not received
// within ReadyTimeout (default 60s), which aborts FX startup.
func (c *consumer) start(_ context.Context) error {
	// Use context.Background() so the stream lifetime is not bounded by the
	// Fx startup context, which expires after app.StartTimeout (~5 minutes).
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.startTime = time.Now()

	c.wg.Add(1)
	go c.streamLoop()

	timeout := c.params.ReadyTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c.log.Infof("Waiting for initial configuration from core agent (timeout: %v)...", timeout)
	if err := c.waitReady(ctx); err != nil {
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

// waitReady blocks until the first config snapshot has been received and applied
func (c *consumer) waitReady(ctx context.Context) error {
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

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Establish connection and stream
		if err := c.connectAndStream(); err != nil {
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
func (c *consumer) connectAndStream() error {
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

		if err := c.handleConfigEvent(event); err != nil {
			c.log.Errorf("Failed to handle config event: %v", err)
		}
	}
}

// handleConfigEvent processes a single config event from the stream
func (c *consumer) handleConfigEvent(event *pb.ConfigEvent) error {
	switch e := event.Event.(type) {
	case *pb.ConfigEvent_Snapshot:
		return c.applySnapshot(e.Snapshot)
	case *pb.ConfigEvent_Update:
		return c.applyUpdate(e.Update)
	default:
		return fmt.Errorf("unknown event type: %T", event.Event)
	}
}

// applySnapshot applies a complete config snapshot
func (c *consumer) applySnapshot(snapshot *pb.ConfigSnapshot) error {
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

	// Signal readiness once the snapshot is fully applied and config mirrored.
	c.readyOnce.Do(func() {
		close(c.readyCh)
		c.ready = true
		duration := time.Since(c.startTime)
		c.timeToFirstSnapshot.Set(duration.Seconds())
		c.log.Infof("Received first config snapshot after %v", duration)
	})

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

	// Detect discontinuity: trigger reconnect so the server sends a fresh snapshot.
	if update.SequenceId != c.lastSeqID+1 {
		return fmt.Errorf("seq_id discontinuity: expected %d, got %d", c.lastSeqID+1, update.SequenceId)
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
		// Only convert integers within float64's exact range (2^53); beyond that,
		// float64 can't represent consecutive integers, so int64 conversion loses precision.
		const maxExact float64 = 1 << 53
		if f >= -maxExact && f <= maxExact && f == math.Trunc(f) {
			return int64(f)
		}
	}

	return result
}
