// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumerimpl implements the configstreamconsumer component.
//
// When enabled, NewComponent dials core, registers with the RAR, fetches the initial
// snapshot, and seeds the global config builder before any other component reads config.
// A background session loop keeps the RAR session alive and re-registers when it is lost.
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

	"github.com/cenkalti/backoff/v7"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// queryTimeout caps RegisterRemoteAgent and stream open; stream Recv uses ctx.
const queryTimeout = 30 * time.Second

// defaultRefreshInterval is the fallback when RAR reports no interval. The consumer seeds the
// config, so it has no config component to read the schema default from.
const defaultRefreshInterval = 10 * time.Second

// noSessionPollInterval is how often streamLoop re-checks for a session being re-minted.
const noSessionPollInterval = time.Second

// seqIDUnset marks "nothing applied on this stream yet" so sequence ID 0 is still accepted.
const seqIDUnset = -1

// ipcTimeout bounds the wait for the core agent to write the IPC auth token and certificate, and
// ipcPollInterval is how often that wait retries.
const (
	ipcTimeout      = 60 * time.Second
	ipcPollInterval = 2 * time.Second
)

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

	// sessionID identifies this consumer's registration with the Remote Agent Registry. It is
	// re-minted whenever RAR drops the session, so every access takes sessionMu.
	sessionID       string
	refreshInterval time.Duration
	sessionMu       sync.RWMutex
	// sessionKick replaces an invalidated session now rather than at the next refresh tick.
	sessionKick chan struct{}

	conn       *grpc.ClientConn
	client     pb.AgentSecureClient
	stream     pb.AgentSecure_StreamConfigEventsClient
	streamLock sync.Mutex

	lastSeqID atomic.Int32

	// Layers this stream has written, keyed by setting. Touched only from the stream goroutine.
	streamedLayers map[string]map[pkgconfigmodel.Source]struct{}

	ready     atomic.Bool
	readyCh   chan struct{}
	readyOnce sync.Once

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startTime time.Time

	timeToFirstSnapshot   telemetry.Gauge
	streamReconnectCount  telemetry.Counter
	lastSeqIDMetric       telemetry.Gauge
	droppedStaleUpdates   telemetry.Counter
	sessionRegistrations  telemetry.Counter
	sessionRefreshFailure telemetry.Counter
}

func (c *consumer) IsActive() bool { return c.ready.Load() }

// noopConsumer is returned when configstream is disabled.
type noopConsumer struct{}

func (noopConsumer) IsActive() bool { return false }

// loadIPCCredentials reads the IPC auth token and client certificate, retrying until timeout.
// A remote agent must never mint either artifact itself, so when the core agent has not written
// them yet — routine when both start together in a container — the only option is to wait.
// Failing immediately exits before FX startup completes, which restarts the container.
func loadIPCCredentials(authTokenPath, certPath string, timeout, interval time.Duration, logger log.Component) (string, *tls.Config, error) {
	deadline := time.Now().Add(timeout)
	waiting := false

	for {
		authToken, err := pkgtoken.LoadAuthTokenFromPath(authTokenPath)
		if err == nil {
			var clientTLS *tls.Config
			clientTLS, err = cert.LoadClientTLSConfigFromPath(certPath)
			if err == nil {
				if waiting {
					logger.Infof("configstreamconsumer: IPC credentials are now available")
				}
				return authToken, clientTLS, nil
			}
		}

		if time.Now().After(deadline) {
			return "", nil, fmt.Errorf("load IPC credentials: gave up after %v: %w", timeout, err)
		}
		if !waiting {
			logger.Infof("configstreamconsumer: waiting up to %v for the core agent to write the IPC credentials (%v)", timeout, err)
			waiting = true
		}
		time.Sleep(interval)
	}
}

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

	// pkglog.NewWrapper avoids the config → configstreamconsumer → log → config FX cycle
	// in system-probe's binary (log.Component depends on config).
	logger := pkglog.NewWrapper(2)

	authToken, clientTLS, err := loadIPCCredentials(
		configstreambootstrap.AuthTokenFilepath(),
		configstreambootstrap.IPCCertFilepath(),
		ipcTimeout, ipcPollInterval, logger,
	)
	if err != nil {
		return Provides{}, err
	}

	// Must drop before snapshot apply, otherwise streamed SourceEnvVar values get wiped too.
	configstreambootstrap.DisableLocalEnvLayer(reqs.Params.ClientName)

	c := &consumer{
		log:       logger,
		telemetry: reqs.Telemetry,
		params:    reqs.Params,
		addr:      net.JoinHostPort(bs.CmdHost, strconv.Itoa(bs.CmdPort)),
		vsockAddr: bs.VSockAddr,
		authToken: authToken,
		clientTLS: clientTLS,
		readyCh:   make(chan struct{}),

		sessionKick:    make(chan struct{}, 1),
		streamedLayers: make(map[string]map[pkgconfigmodel.Source]struct{}),
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

	c.wg.Add(2)
	go c.sessionLoop()
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

// session returns the current RAR session ID, or "" while re-registration is pending.
func (c *consumer) session() string {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.sessionID
}

func (c *consumer) setSession(sessionID string, refreshInterval time.Duration) {
	c.sessionMu.Lock()
	c.sessionID = sessionID
	c.refreshInterval = refreshInterval
	c.sessionMu.Unlock()
}

// invalidateSession makes sessionLoop mint a new session on its next tick.
func (c *consumer) invalidateSession(reason string) {
	c.sessionMu.Lock()
	dropped := c.sessionID != ""
	if dropped {
		c.log.Warnf("configstreamconsumer[%s]: dropping session %s: %s", c.params.ClientName, c.sessionID, reason)
		c.sessionID = ""
	}
	c.sessionMu.Unlock()

	if dropped {
		// The open stream still carries the dead session_id and the server refreshes it for
		// as long as it holds the stream, so leaving it up would keep a second registration
		// alive alongside the replacement. Drop it and let streamLoop redial.
		c.resetStream()
		select {
		case c.sessionKick <- struct{}{}:
		default:
		}
	}
}

// resetStream tears down the active stream so streamLoop reconnects with a fresh session.
func (c *consumer) resetStream() {
	c.streamLock.Lock()
	defer c.streamLock.Unlock()
	if c.stream != nil {
		_ = c.stream.CloseSend()
		c.stream = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// sessionRejected distinguishes RAR authoritatively refusing the session from a transport
// hiccup, which says nothing about whether the session is still registered.
func sessionRejected(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.NotFound, codes.PermissionDenied, codes.Unauthenticated:
		return true
	}
	return false
}

func (c *consumer) currentRefreshInterval() time.Duration {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	if c.refreshInterval <= 0 {
		return defaultRefreshInterval
	}
	return c.refreshInterval
}

func newRegistrationBackoff() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = time.Minute
	bo.Reset()
	return bo
}

// registerOnce mints a new session over a short-lived connection.
func (c *consumer) registerOnce() error {
	client, conn, err := helper.NewAgentSecureClient(c.addr, c.authToken, c.clientTLS, c.vsockAddr, c.log)
	if err != nil {
		return err
	}
	defer conn.Close()

	sessionID, refreshInterval, err := helper.RegisterRemoteAgent(c.ctx, client, helper.RegistrationRequest{
		Flavor:      flavor.GetFlavor(),
		DisplayName: c.params.ClientName,
		// Sentinel URI: the consumer registers no services, so core never dials back.
		APIEndpointURI: "https://configstream-consumer/" + c.params.ClientName,
	}, queryTimeout, defaultRefreshInterval, c.log)
	if err != nil {
		return err
	}
	c.setSession(sessionID, refreshInterval)
	c.sessionRegistrations.Inc()
	return nil
}

// registerWithBackoff retries forever until ctx is canceled, with no fallback.
func (c *consumer) registerWithBackoff() error {
	bo := newRegistrationBackoff()
	for attempt := 1; ; attempt++ {
		err := c.registerOnce()
		if err == nil {
			return nil
		}
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
		// NextBackOff never returns backoff.Stop when MaxElapsedTime is 0 (the default).
		next := bo.NextBackOff()
		c.log.Warnf("configstreamconsumer[%s]: register attempt %d failed (%v); retrying in %s", c.params.ClientName, attempt, err, next)
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(next):
		}
	}
}

// sessionLoop keeps the RAR session alive. An open config stream does not count as activity,
// so without a periodic refresh the reaper evicts the session and every later reconnect is
// rejected with PermissionDenied. Falls back to re-registering once the session is gone.
func (c *consumer) sessionLoop() {
	defer c.wg.Done()

	var (
		client pb.AgentSecureClient
		conn   *grpc.ClientConn
	)
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()
	dropConn := func() {
		if conn != nil {
			_ = conn.Close()
		}
		client, conn = nil, nil
	}

	bo := newRegistrationBackoff()
	ticker := time.NewTicker(c.currentRefreshInterval())
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		case <-c.sessionKick:
		}

		if c.session() == "" {
			if err := c.registerOnce(); err != nil {
				next := bo.NextBackOff()
				c.log.Warnf("configstreamconsumer[%s]: re-registration failed (%v); retrying in %s", c.params.ClientName, err, next)
				ticker.Reset(next)
				continue
			}
			c.log.Infof("configstreamconsumer[%s]: re-registered with RAR (session_id=%s)", c.params.ClientName, c.session())
			bo.Reset()
			ticker.Reset(c.currentRefreshInterval())
			continue
		}

		if client == nil {
			var err error
			if client, conn, err = helper.NewAgentSecureClient(c.addr, c.authToken, c.clientTLS, c.vsockAddr, c.log); err != nil {
				client, conn = nil, nil
				next := bo.NextBackOff()
				c.log.Warnf("configstreamconsumer[%s]: failed to connect for session refresh (%v); retrying in %s", c.params.ClientName, err, next)
				ticker.Reset(next)
				continue
			}
		}

		if err := c.refreshSession(client); err != nil {
			c.sessionRefreshFailure.Inc()
			// The connection may itself be the problem, so redial on the next tick.
			dropConn()
			if sessionRejected(err) {
				c.invalidateSession(fmt.Sprintf("refresh rejected: %v", err))
			} else {
				c.log.Warnf("configstreamconsumer[%s]: session refresh failed (%v); retrying", c.params.ClientName, err)
			}
			ticker.Reset(bo.NextBackOff())
			continue
		}
		bo.Reset()
		ticker.Reset(c.currentRefreshInterval())
	}
}

func (c *consumer) refreshSession(client pb.AgentSecureClient) error {
	ctx, cancel := context.WithTimeout(c.ctx, queryTimeout)
	defer cancel()
	_, err := client.RefreshRemoteAgent(ctx, &pb.RefreshRemoteAgentRequest{SessionId: c.session()})
	return err
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

		// sessionLoop is re-registering; dialing now would only earn another rejection.
		if c.session() == "" {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(noSessionPollInterval):
			}
			continue
		}

		if err := c.connectAndStream(); err != nil {
			if err == context.Canceled || c.ctx.Err() != nil {
				return
			}
			// A rejected session_id stays rejected until a new one is minted.
			if sessionRejected(err) {
				c.invalidateSession(err.Error())
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

	md := metadata.New(map[string]string{"session_id": c.session()})
	ctxWithMetadata := metadata.NewOutgoingContext(c.ctx, md)

	// Sequence IDs are per core-agent process, so they carry no meaning across a reconnect.
	// The leading snapshot of a new subscription is authoritative whatever its sequence ID.
	c.lastSeqID.Store(seqIDUnset)

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
		c.log.Warnf("Ignoring stale snapshot (seq_id: %d <= %d)", snapshot.SequenceId, c.lastSeqID.Load())
		c.droppedStaleUpdates.Inc()
		return nil
	}

	c.log.Infof("Applying config snapshot (seq_id: %d, settings: %d)", snapshot.SequenceId, len(snapshot.Settings))

	settings := make([]pkgconfigmodel.DirectSetting, 0, len(snapshot.Settings))
	fresh := make(map[string]map[pkgconfigmodel.Source]struct{}, len(snapshot.Settings))
	for _, setting := range snapshot.Settings {
		ds := toDirectSetting(setting)
		settings = append(settings, ds)
		if fresh[ds.Key] == nil {
			fresh[ds.Key] = make(map[pkgconfigmodel.Source]struct{}, 1)
		}
		fresh[ds.Key][ds.Source] = struct{}{}
	}

	cfg := configstreambootstrap.Config()
	// The first snapshot seeds a config nothing has read yet; a later one replaces a config the
	// process is already running on, so its changes have to be broadcast.
	cfg.DirectBulkSet(settings, c.ready.Load())

	// A snapshot is the sender's entire state, so a layer the stream wrote earlier that the
	// snapshot no longer claims has to be retracted. Left in place it would outrank the snapshot's
	// own entry for that key and resurrect a value removed while this client was disconnected.
	// Retracted after the bulk set so each notification already carries the value that now wins.
	for key, sources := range c.streamedLayers {
		for source := range sources {
			if _, stillClaimed := fresh[key][source]; stillClaimed {
				continue
			}
			c.log.Debugf("Snapshot retracts stale layer (key: %s, source: %s)", key, source)
			cfg.UnsetForSource(key, source)
		}
	}
	c.streamedLayers = fresh

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

// recordLayer notes that the stream put key into source, so a later snapshot can retract the
// layer if it stops claiming it.
func (c *consumer) recordLayer(key string, source pkgconfigmodel.Source) {
	if c.streamedLayers == nil {
		c.streamedLayers = make(map[string]map[pkgconfigmodel.Source]struct{})
	}
	if c.streamedLayers[key] == nil {
		c.streamedLayers[key] = make(map[pkgconfigmodel.Source]struct{}, 1)
	}
	c.streamedLayers[key][source] = struct{}{}
}

// toDirectSetting decodes a streamed setting into the form the config builder takes.
func toDirectSetting(setting *pb.ConfigSetting) pkgconfigmodel.DirectSetting {
	return pkgconfigmodel.DirectSetting{
		Key:    setting.Key,
		Value:  pbValueToGo(setting.Value),
		Source: pkgconfigmodel.Source(setting.Source),
	}
}

// pbValueToGo converts a protobuf Value to a Go value. structpb has no integer type, so numbers
// arrive as float64; narrowing is left to the declared default type.
func pbValueToGo(v *structpb.Value) any {
	if v == nil {
		return nil
	}
	return v.AsInterface()
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

	cfg := configstreambootstrap.Config()

	if update.Setting.UnsetSource == "" {
		c.log.Debugf("Applying config update (seq_id: %d, key: %s)", update.SequenceId, update.Setting.Key)
		setting := toDirectSetting(update.Setting)
		// Updates never carry env-var-sourced settings, so Set's guardrail is not in the way and
		// registered receivers still get notified.
		cfg.Set(setting.Key, setting.Value, setting.Source)
		c.recordLayer(setting.Key, setting.Source)
	} else {
		c.log.Debugf("Applying config unset (seq_id: %d, key: %s, cleared source: %s)", update.SequenceId, update.Setting.Key, update.Setting.UnsetSource)
		// Seeding the layer fallen back to is not optional: snapshots only carry the merged view, so
		// this config has no lower layer of its own to fall back to. It is seeded before the unset
		// rather than after, so the unset resolves onto it and notifies local receivers once with
		// the final value instead of transiently with the default. Empty only for an undeclared key.
		if update.Setting.Source != "" {
			cfg.DirectBulkSet([]pkgconfigmodel.DirectSetting{toDirectSetting(update.Setting)}, false)
			c.recordLayer(update.Setting.Key, pkgconfigmodel.Source(update.Setting.Source))
		}
		cleared := pkgconfigmodel.Source(update.Setting.UnsetSource)
		cfg.UnsetForSource(update.Setting.Key, cleared)
		if sources := c.streamedLayers[update.Setting.Key]; sources != nil {
			delete(sources, cleared)
			if len(sources) == 0 {
				delete(c.streamedLayers, update.Setting.Key)
			}
		}
	}

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
	c.sessionRegistrations = c.telemetry.NewCounter(
		"configstream_consumer",
		"session_registrations",
		[]string{},
		"Number of Remote Agent Registry registrations, including re-registrations after a lost session",
	)
	c.sessionRefreshFailure = c.telemetry.NewCounter(
		"configstream_consumer",
		"session_refresh_failures",
		[]string{},
		"Number of failed Remote Agent Registry session refreshes",
	)
}
