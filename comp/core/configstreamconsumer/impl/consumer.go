// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumerimpl implements the configstreamconsumer component.
//
// When enabled, NewComponent dials core, registers with the RAR, fetches the initial
// snapshot, and seeds the global config builder before any other component reads config.
// Global-builder writes are delegated to pkg/configstreambootstrap because the
// pkgconfigusage depguard blocks pkg/config/setup imports from comp/.
package configstreamconsumerimpl

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v6"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// queryTimeout caps RegisterRemoteAgent and stream open; stream Recv uses ctx.
const queryTimeout = 30 * time.Second

// Requires defines the dependencies for the configstreamconsumer component
type Requires struct {
	compdef.In

	Lifecycle compdef.Lifecycle
	Telemetry telemetry.Component
	Params    configstreamconsumer.Params
}

// Provides defines the output of the configstreamconsumer component
type Provides struct {
	compdef.Out

	Comp configstreamconsumer.Component
}

// consumer implements the configstreamconsumer.Component interface
type consumer struct {
	log       log.Component
	telemetry telemetry.Component
	params    configstreamconsumer.Params

	addr      string
	vsockAddr string
	authToken string
	clientTLS *tls.Config
	sessionID string

	conn       *grpc.ClientConn
	client     pb.AgentSecureClient
	stream     pb.AgentSecure_StreamConfigEventsClient
	streamLock sync.Mutex

	lastSeqID atomic.Int32

	ready     atomic.Bool
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

func (c *consumer) IsActive() bool { return c.ready.Load() }

// noopConsumer is returned when configstream is disabled.
type noopConsumer struct{}

func (noopConsumer) IsActive() bool { return false }

// NewComponent returns a no-op when configstream is disabled; otherwise it blocks until
// the first snapshot lands (or ReadyTimeout) before returning.
func NewComponent(reqs Requires) (Provides, error) {
	if reqs.Params.ClientName == "" {
		return Provides{}, errors.New("configstreamconsumer: ClientName is required")
	}

	if !isEnabled(reqs.Params.CLIConfigPath) {
		return Provides{Comp: noopConsumer{}}, nil
	}

	bs := readSettings(reqs.Params.CLIConfigPath)
	if !bs.RARRegistryEnabled {
		return Provides{}, fmt.Errorf("configstream consumer requires remote_agent.registry.enabled=true; refusing to start %s without RAR", reqs.Params.ClientName)
	}

	configstreambootstrap.SeedGlobalBuilder(bs, resolvedConfigFile(reqs.Params.CLIConfigPath))

	authToken, err := pkgtoken.LoadAuthTokenFromPath(configstreambootstrap.AuthTokenFilepath())
	if err != nil {
		return Provides{}, fmt.Errorf("load auth token: %w", err)
	}
	clientTLS, err := cert.LoadClientTLSConfigFromPath(configstreambootstrap.IPCCertFilepath())
	if err != nil {
		return Provides{}, fmt.Errorf("load IPC cert: %w", err)
	}

	// Must drop before snapshot apply, otherwise streamed SourceEnvVar values get wiped too.
	configstreambootstrap.DisableLocalEnvLayer(reqs.Params.ClientName)

	c := &consumer{
		// pkglog.NewWrapper avoids the config → configstreamconsumer → log → config FX cycle
		// in system-probe's binary (log.Component depends on config).
		log:       pkglog.NewWrapper(2),
		telemetry: reqs.Telemetry,
		params:    reqs.Params,
		addr:      net.JoinHostPort(bs.CmdHost, strconv.Itoa(bs.CmdPort)),
		vsockAddr: bs.VSockAddr,
		authToken: authToken,
		clientTLS: clientTLS,
		readyCh:   make(chan struct{}),
	}
	c.initMetrics()

	if err := c.start(context.Background()); err != nil {
		return Provides{}, err
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStop: c.stop})
	return Provides{Comp: c}, nil
}

func (c *consumer) start(_ context.Context) error {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.startTime = time.Now()

	if err := c.registerWithBackoff(); err != nil {
		return err
	}

	c.wg.Add(1)
	go c.streamLoop()

	timeout := c.params.ReadyTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c.log.Infof("configstreamconsumer[%s]: waiting for initial configuration (timeout: %v)...", c.params.ClientName, timeout)
	if err := c.waitReady(ctx); err != nil {
		c.cancel()
		c.wg.Wait()
		return fmt.Errorf("waiting for initial config snapshot: %w", err)
	}
	c.log.Infof("configstreamconsumer[%s]: initial configuration received.", c.params.ClientName)
	return nil
}

// registerWithBackoff retries forever until ctx is canceled, with no fallback.
func (c *consumer) registerWithBackoff() error {
	// Sentinel URI: the consumer registers no services, so core never dials back.
	apiEndpointURI := "https://configstream-consumer/" + c.params.ClientName

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = time.Minute
	bo.Reset()
	for attempt := 1; ; attempt++ {
		client, conn, dialErr := helper.NewAgentSecureClient(c.addr, c.authToken, c.clientTLS, c.vsockAddr, c.log)
		if dialErr == nil {
			sessionID, _, regErr := helper.RegisterRemoteAgent(c.ctx, client, helper.RegistrationRequest{
				Flavor:         flavor.GetFlavor(),
				DisplayName:    c.params.ClientName,
				APIEndpointURI: apiEndpointURI,
			}, queryTimeout, 0, c.log)
			if regErr == nil {
				c.sessionID = sessionID
				_ = conn.Close()
				return nil
			}
			_ = conn.Close()
			dialErr = regErr
		}
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
		// NextBackOff never returns backoff.Stop when MaxElapsedTime is 0 (the default).
		next := bo.NextBackOff()
		c.log.Warnf("configstreamconsumer[%s]: register attempt %d failed (%v); retrying in %s", c.params.ClientName, attempt, dialErr, next)
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(next):
		}
	}
}

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

func (c *consumer) waitReady(ctx context.Context) error {
	select {
	case <-c.readyCh:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for config snapshot: %w", ctx.Err())
	}
}

func (c *consumer) streamLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

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

func (c *consumer) connectAndStream() error {
	client, conn, err := helper.NewAgentSecureClient(c.addr, c.authToken, c.clientTLS, c.vsockAddr, c.log)
	if err != nil {
		return fmt.Errorf("failed to connect to core agent: %w", err)
	}
	defer conn.Close()

	c.streamLock.Lock()
	c.conn = conn
	c.client = client
	c.streamLock.Unlock()

	md := metadata.New(map[string]string{"session_id": c.sessionID})
	ctxWithMetadata := metadata.NewOutgoingContext(c.ctx, md)

	stream, err := c.client.StreamConfigEvents(ctxWithMetadata, &pb.ConfigStreamRequest{Name: c.params.ClientName})
	if err != nil {
		return fmt.Errorf("failed to start config stream: %w", err)
	}

	c.streamLock.Lock()
	c.stream = stream
	c.streamLock.Unlock()

	c.log.Infof("configstreamconsumer[%s]: stream established", c.params.ClientName)

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
			return fmt.Errorf("config event error: %w", err)
		}
	}
}

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

func (c *consumer) applySnapshot(snapshot *pb.ConfigSnapshot) error {
	if snapshot.SequenceId <= c.lastSeqID.Load() {
		c.log.Errorf("Received snapshot with seq_id %d <= current %d; the core agent may have restarted. "+
			"This sub-process must be restarted to accept a new configuration.", snapshot.SequenceId, c.lastSeqID.Load())
		c.droppedStaleUpdates.Inc()
		return nil
	}

	c.log.Infof("Applying config snapshot (seq_id: %d, settings: %d)", snapshot.SequenceId, len(snapshot.Settings))

	for _, setting := range snapshot.Settings {
		configstreambootstrap.ApplySetting(setting.Key, setting.Value, setting.Source)
	}
	c.lastSeqID.Store(snapshot.SequenceId)
	c.lastSeqIDMetric.Set(float64(snapshot.SequenceId))

	c.readyOnce.Do(func() {
		close(c.readyCh)
		c.ready.Store(true)
		duration := time.Since(c.startTime)
		c.timeToFirstSnapshot.Set(duration.Seconds())
		c.log.Infof("configstreamconsumer[%s]: first snapshot applied after %v", c.params.ClientName, duration)
	})

	return nil
}

func (c *consumer) applyUpdate(update *pb.ConfigUpdate) error {
	if update.SequenceId <= c.lastSeqID.Load() {
		c.log.Warnf("Ignoring stale update (seq_id: %d <= %d)", update.SequenceId, c.lastSeqID.Load())
		c.droppedStaleUpdates.Inc()
		return nil
	}

	if update.SequenceId != c.lastSeqID.Load()+1 {
		return fmt.Errorf("seq_id discontinuity: expected %d, got %d", c.lastSeqID.Load()+1, update.SequenceId)
	}

	c.log.Debugf("Applying config update (seq_id: %d, key: %s)", update.SequenceId, update.Setting.Key)

	configstreambootstrap.ApplySetting(update.Setting.Key, update.Setting.Value, update.Setting.Source)
	c.lastSeqID.Store(update.SequenceId)
	c.lastSeqIDMetric.Set(float64(update.SequenceId))

	return nil
}

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
