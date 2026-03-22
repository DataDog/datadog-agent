// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package opamp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
)

// extension is the DDOT OpAmp extension. It wraps the opamp-go client and
// declares additional capabilities beyond what the upstream opampextension
// provides: ReportsHeartbeat, AcceptsOpAMPConnectionSettings,
// ReportsConnectionSettingsStatus, ReportsOwnMetrics, and AcceptsRemoteConfig.
//
// speky:DDOT#OTELCOL025 speky:DDOT#OTELCOL026 speky:DDOT#OTELCOL027
// speky:DDOT#OTELCOL028 speky:DDOT#OTELCOL029 speky:DDOT#OTELCOL034
type ddotOpampExtension struct {
	cfg    *Config
	set    extension.Settings
	logger *zap.Logger
	client client.OpAMPClient

	// statusCh receives component status changes for health reporting.
	statusCh chan *componentstatus.Event

	cancelCtx  context.CancelFunc
	ownMetrics *ownMetricsReporter

	// remoteCfg is the hot-reload provider; nil when AcceptsRemoteConfig is disabled.
	remoteCfg *RemoteConfigProvider

	// mu protects lastRemoteCfgHash.
	mu                sync.Mutex
	lastRemoteCfgHash []byte
}

var (
	_ extension.Extension     = (*ddotOpampExtension)(nil)
	_ componentstatus.Watcher = (*ddotOpampExtension)(nil)
)

func newExtension(set extension.Settings, cfg *Config, remoteCfg *RemoteConfigProvider) (*ddotOpampExtension, error) {
	buildInfo := set.BuildInfo
	// Make a copy so we can adjust capabilities without mutating the shared default.
	cfgCopy := *cfg
	if remoteCfg == nil {
		// Cannot declare AcceptsRemoteConfig without a hot-reload provider.
		cfgCopy.Capabilities.AcceptsRemoteConfig = false
	}
	return &ddotOpampExtension{
		cfg:        &cfgCopy,
		set:        set,
		logger:     set.Logger,
		statusCh:   make(chan *componentstatus.Event, 16),
		ownMetrics: newOwnMetricsReporter(set.Logger, buildInfo.Command, buildInfo.Version),
		remoteCfg:  remoteCfg,
	}, nil
}

// Start implements extension.Extension.
func (e *ddotOpampExtension) Start(ctx context.Context, host component.Host) error {
	if e.cfg.Server == nil {
		return fmt.Errorf("opamp: server configuration is required")
	}

	tlsCfg, err := e.cfg.Server.GetTLSConfig(ctx)
	if err != nil {
		return fmt.Errorf("opamp: building TLS config: %w", err)
	}

	header := http.Header{}
	for k, v := range e.cfg.Server.GetHeaders() {
		header.Set(k, string(v))
	}

	instanceID, err := e.resolveInstanceUID()
	if err != nil {
		return fmt.Errorf("opamp: resolving instance UID: %w", err)
	}

	caps := protobufs.AgentCapabilities(e.cfg.Capabilities.toAgentCapabilities())

	lifetimeCtx, cancel := context.WithCancel(context.Background())
	e.cancelCtx = cancel

	e.client = e.cfg.Server.GetClient(e.logger)

	settings := types.StartSettings{
		Header:         header,
		TLSConfig:      tlsCfg,
		OpAMPServerURL: e.cfg.Server.GetEndpoint(),
		InstanceUid:    types.InstanceUid(instanceID),
		Callbacks: types.Callbacks{
			OnConnect: func(_ context.Context) {
				e.logger.Debug("Connected to the OpAMP server")
			},
			OnConnectFailed: func(_ context.Context, err error) {
				e.logger.Error("Failed to connect to the OpAMP server", zap.Error(err))
			},
			OnError: func(_ context.Context, err *protobufs.ServerErrorResponse) {
				e.logger.Error("OpAMP server returned an error response", zap.String("message", err.ErrorMessage))
			},
			GetEffectiveConfig: func(_ context.Context) (*protobufs.EffectiveConfig, error) {
				return &protobufs.EffectiveConfig{
					ConfigMap: &protobufs.AgentConfigMap{
						ConfigMap: map[string]*protobufs.AgentConfigFile{},
					},
				}, nil
			},
			OnOpampConnectionSettings: e.onOpampConnectionSettings,
			OnMessage:                 e.onMessage,
		},
	}

	if err := e.client.SetAgentDescription(e.buildAgentDescription()); err != nil {
		return fmt.Errorf("opamp: setting agent description: %w", err)
	}
	// SetHealth and SetAvailableComponents must be called before SetCapabilities when
	// the corresponding capabilities are declared; the opamp-go client validates that the
	// backing objects are non-nil before accepting the capabilities bitmask.
	if e.cfg.Capabilities.ReportsHealth {
		if err := e.client.SetHealth(&protobufs.ComponentHealth{Healthy: true}); err != nil {
			return fmt.Errorf("opamp: setting initial health: %w", err)
		}
	}
	if e.cfg.Capabilities.ReportsAvailableComponents {
		// Use an empty component set (SHA256 of empty string as hash). A future
		// improvement can enumerate the actual factories from the host.
		emptyHash := sha256.Sum256(nil)
		if err := e.client.SetAvailableComponents(&protobufs.AvailableComponents{
			Hash: emptyHash[:],
		}); err != nil {
			return fmt.Errorf("opamp: setting available components: %w", err)
		}
	}
	if err := e.client.SetCapabilities(&caps); err != nil {
		return fmt.Errorf("opamp: setting capabilities: %w", err)
	}
	if err := e.client.Start(ctx, settings); err != nil {
		return fmt.Errorf("opamp: starting client: %w", err)
	}

	// Process ongoing health status changes in the background.
	go e.runHealthLoop(lifetimeCtx, host)

	return nil
}

// Shutdown implements extension.Extension.
func (e *ddotOpampExtension) Shutdown(ctx context.Context) error {
	if e.cancelCtx != nil {
		e.cancelCtx()
	}
	if e.ownMetrics != nil {
		e.ownMetrics.shutdown()
	}
	if e.client != nil {
		return e.client.Stop(ctx)
	}
	return nil
}

// ComponentStatusChanged implements componentstatus.Watcher.
// The collector calls this when any component changes state.
func (e *ddotOpampExtension) ComponentStatusChanged(
	_ *componentstatus.InstanceID,
	event *componentstatus.Event,
) {
	if event.Status() == componentstatus.StatusStarting {
		return // ignore transient starting events
	}
	select {
	case e.statusCh <- event:
	default:
		// Drop if the channel is full to avoid blocking the caller.
	}
}

// runHealthLoop translates component status events into OpAMP health reports.
func (e *ddotOpampExtension) runHealthLoop(ctx context.Context, _ component.Host) {
	if !e.cfg.Capabilities.ReportsHealth {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-e.statusCh:
			if !ok {
				return
			}
			healthy := ev.Status() == componentstatus.StatusOK
			h := &protobufs.ComponentHealth{Healthy: healthy}
			if ev.Err() != nil {
				h.LastError = ev.Err().Error()
			}
			if err := e.client.SetHealth(h); err != nil {
				e.logger.Warn("Could not report health to OpAMP server", zap.Error(err))
			}
		}
	}
}

// onOpampConnectionSettings is the callback called by the opamp-go client when
// the server pushes OpAMPConnectionSettings (TLS cert, heartbeat interval, etc.).
//
// If the pushed TLS certificate is present but malformed, it returns an error so
// the client reports ConnectionSettingsStatus=FAILED back to the server (T028).
// The heartbeat interval change is handled automatically by the opamp-go client
// before this callback is invoked (T020).
//
// speky:DDOT#OTELCOL034
func (e *ddotOpampExtension) onOpampConnectionSettings(
	_ context.Context,
	settings *protobufs.OpAMPConnectionSettings,
) error {
	if settings == nil {
		return nil
	}
	cert := settings.Certificate
	if cert == nil {
		return nil
	}
	// Validate the certificate and key. If parsing fails, the opamp-go client
	// will report FAILED status to the server (see receivedprocessor.go).
	if _, err := tls.X509KeyPair(cert.Cert, cert.PrivateKey); err != nil {
		return fmt.Errorf("invalid TLS certificate: %w", err)
	}
	// Certificate is valid: store it for the next reconnection.
	// (Actual reconnection with the new cert is handled by the opamp-go client.)
	e.logger.Info("OpAMP server pushed a new TLS certificate; will use on next reconnection")
	return nil
}

// buildAgentDescription constructs the AgentDescription from the extension
// settings and the agent_description config injected by the converter.
func (e *ddotOpampExtension) buildAgentDescription() *protobufs.AgentDescription {
	buildInfo := e.set.BuildInfo

	// Identifying attributes: service.name and service.version from build info.
	identifying := []*protobufs.KeyValue{
		kv("service.name", buildInfo.Command),
		kv("service.version", buildInfo.Version),
	}

	// Non-identifying attributes: whatever the converter injected.
	var nonIdentifying []*protobufs.KeyValue
	for k, v := range e.cfg.AgentDescription.NonIdentifyingAttributes {
		nonIdentifying = append(nonIdentifying, kv(k, v))
	}

	return &protobufs.AgentDescription{
		IdentifyingAttributes:    identifying,
		NonIdentifyingAttributes: nonIdentifying,
	}
}

// resolveInstanceUID parses the instance UID from the config (injected by the
// converter from the persisted otel-instance-uid file) or generates a fallback.
func (e *ddotOpampExtension) resolveInstanceUID() (uuid.UUID, error) {
	if e.cfg.InstanceUID != "" {
		id, err := uuid.Parse(e.cfg.InstanceUID)
		if err == nil {
			return id, nil
		}
		e.logger.Warn("instance_uid is not a valid UUID; generating a new one", zap.Error(err))
	}
	return uuid.NewV7()
}

// kv is a convenience constructor for protobufs.KeyValue string values.
func kv(key, value string) *protobufs.KeyValue {
	return &protobufs.KeyValue{
		Key: key,
		Value: &protobufs.AnyValue{
			Value: &protobufs.AnyValue_StringValue{StringValue: value},
		},
	}
}

// onMessage is the callback invoked by the opamp-go client when the server
// sends a message that requires agent-side processing (e.g. OwnMetrics
// connection settings, RemoteConfig).
//
// speky:DDOT#OTELCOL031 speky:DDOT#OTELCOL032
func (e *ddotOpampExtension) onMessage(ctx context.Context, msg *types.MessageData) {
	if msg.OwnMetricsConnSettings != nil && e.cfg.Capabilities.ReportsOwnMetrics {
		e.ownMetrics.applySettings(msg.OwnMetricsConnSettings)
	}
	if msg.RemoteConfig != nil && e.cfg.Capabilities.AcceptsRemoteConfig && e.remoteCfg != nil {
		e.applyRemoteConfig(ctx, msg.RemoteConfig)
	}
}

// applyRemoteConfig processes an AgentRemoteConfig pushed by the OpAMP server.
// It extracts the YAML body from the config map, pushes it to the
// RemoteConfigProvider (triggering a collector hot-reload), and reports the
// resulting status back to the server.  Identical successive pushes (same
// hash) are acknowledged as APPLIED without restarting the pipeline.
//
// speky:DDOT#OTELCOL031
func (e *ddotOpampExtension) applyRemoteConfig(ctx context.Context, rc *protobufs.AgentRemoteConfig) {
	hash := rc.ConfigHash

	// Idempotent: if the server resends the same config, re-report APPLIED
	// without restarting the pipeline.
	e.mu.Lock()
	sameHash := bytes.Equal(hash, e.lastRemoteCfgHash)
	e.mu.Unlock()
	if sameHash {
		if err := e.client.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
			LastRemoteConfigHash: hash,
			Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
		}); err != nil {
			e.logger.Warn("Could not send RemoteConfigStatus (idempotent)", zap.Error(err))
		}
		return
	}

	// Extract the YAML body from the first entry in the config map.
	var yamlContent []byte
	if rc.Config != nil {
		for _, v := range rc.Config.ConfigMap {
			if v != nil {
				yamlContent = v.Body
				break
			}
		}
	}

	// Push to the provider, which signals the collector to reload.
	e.remoteCfg.Push(yamlContent)

	e.mu.Lock()
	e.lastRemoteCfgHash = hash
	e.mu.Unlock()

	if err := e.client.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
		LastRemoteConfigHash: hash,
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
	}); err != nil {
		e.logger.Warn("Could not send RemoteConfigStatus", zap.Error(err))
	}

	// Ask the client to send an updated EffectiveConfig after the reload.
	if err := e.client.UpdateEffectiveConfig(ctx); err != nil {
		e.logger.Warn("Could not update effective config after remote config apply", zap.Error(err))
	}
}
