// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package output

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logimpl "github.com/DataDog/datadog-agent/comp/core/log/impl"
	secretnoop "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	forwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/impl"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/runner"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

type isolatedConfig struct {
	configmodel.BuildableConfig
	start time.Time
}

func (c *isolatedConfig) StartTime() time.Time { return c.start }

type sysprobeConfig struct{ configmodel.BuildableConfig }

func (*sysprobeConfig) SysProbeObject() *sysconfig.Config { return &sysconfig.Config{} }

// NewConfig copies only schema defaults, never the operator's live Agent
// configuration or DD_* endpoint variables. Every destination is then explicit.
func NewConfig() config.Component {
	cfg := nodetreemodel.NewNodeTreeConfig("eudm-simulator", "EUDM_SIMULATOR_INTERNAL", strings.NewReplacer(".", "_"))
	var copyDefaults func(string, any)
	copyDefaults = func(prefix string, value any) {
		if children, ok := value.(map[string]interface{}); ok && len(children) > 0 {
			for key, child := range children {
				path := key
				if prefix != "" {
					path = prefix + "." + key
				}
				copyDefaults(path, child)
			}
			return
		}
		if prefix != "" {
			cfg.SetDefault(prefix, value)
		}
	}
	copyDefaults("", setup.SystemProbe().AllSettingsBySource()[configmodel.SourceDefault])
	copyDefaults("", setup.Datadog().AllSettingsBySource()[configmodel.SourceDefault])
	for _, key := range append(setup.SystemProbe().AllKeysLowercased(), setup.Datadog().AllKeysLowercased()...) {
		if !cfg.IsKnown(key) {
			cfg.SetDefault(key, nil)
		}
	}
	cfg.BuildSchema()
	return &isolatedConfig{BuildableConfig: cfg, start: time.Now()}
}

// Hostname implements the standard component using an already captured or
// replay-assigned identity, without resolving metadata from the local host.
type Hostname string

func (h Hostname) Get(context.Context) (string, error) { return string(h), nil }
func (h Hostname) GetSafe(context.Context) string      { return string(h) }
func (h Hostname) GetWithProvider(context.Context) (hostname.Data, error) {
	return hostname.Data{Hostname: string(h), Provider: "configuration"}, nil
}

type processForwarders struct{ process, connections *forwarder.DefaultForwarder }

func (f processForwarders) GetProcessForwarder() forwarderdef.Component   { return f.process }
func (f processForwarders) GetRTProcessForwarder() forwarderdef.Component { return f.process }
func (f processForwarders) GetConnectionsForwarder() connectionsforwarder.Component {
	return f.connections
}

type metadataRouter struct {
	forwarderdef.Forwarder
	metadata forwarderdef.Forwarder
}

func (f metadataRouter) SubmitMetadata(payloads transaction.BytesPayloads, headers http.Header) error {
	return f.metadata.SubmitMetadata(payloads, headers)
}

func (f metadataRouter) SubmitHostMetadata(payloads transaction.BytesPayloads, headers http.Header) error {
	return f.metadata.SubmitHostMetadata(payloads, headers)
}

// Pipeline owns the common delivery build for both captured operating systems.
type Pipeline struct {
	Config     config.Component
	Serializer *serializer.Serializer
	Submitter  *runner.CheckSubmitter
	Events     *eventplatform.IsolatedForwarder
	Tracker    *transaction.DeliveryTracker
	forwarders []*forwarder.DefaultForwarder
	cancel     context.CancelFunc
}

// Options are installed before any delivery starts. Replay may constrain the
// Agent queues to bound outstanding replay work.
type Options struct {
	QueueCapacity int
}

// New constructs isolated Agent serializers and forwarders. A Recorder prevents
// intake networking in tests. Replay supplies a staging-guarded transport
// and keeps the Agent's ordinary retry policy.
func New(ctx context.Context, destinations map[safety.Destination][]string, apiKey string, transport http.RoundTripper, options ...Options) (*Pipeline, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("delivery requires an API key")
	}
	resolved, err := (safety.Config{Site: safety.Site, Endpoints: destinations}).Resolve(func(string) string { return "" })
	if err != nil {
		return nil, err
	}
	destinations = resolved
	ctx, cancel := context.WithCancel(ctx)
	p := &Pipeline{Config: NewConfig(), Tracker: transaction.NewDeliveryTracker(), cancel: cancel}
	var settings Options
	if len(options) > 0 {
		settings = options[0]
	}
	if settings.QueueCapacity < 0 {
		cancel()
		return nil, errors.New("queue capacity cannot be negative")
	}
	success := false
	defer func() {
		if !success {
			p.Close()
		}
	}()
	set := func(key string, value any) { p.Config.Set(key, value, configmodel.SourceAgentRuntime) }
	for key, value := range map[string]any{"site": safety.Site, "api_key": apiKey, "infrastructure_mode": "end_user_device", "network_config.direct_send": false, "process_config.process_collection.enabled": true, "forwarder_storage_max_size_in_bytes": 0, "forwarder_stop_timeout": 5, "forwarder_stop_wait_for_inflight": true, "enable_payloads.series": true, "enable_payloads.json_to_v1_intake": true, "software_inventory.enabled": true} {
		set(key, value)
	}
	if settings.QueueCapacity > 0 {
		set("process_config.queue_size", settings.QueueCapacity)
	}
	compressor := selector.FromConfig(p.Config)
	logger := logimpl.NewTemporaryLoggerWithoutInit()
	if transport == nil {
		transport = stagingTransport{base: forwarder.NewHTTPTransport(p.Config, p.Config.GetInt("forwarder_num_workers"), logger)}
	}
	byStream := map[safety.Destination]*forwarder.DefaultForwarder{}
	for _, stream := range []safety.Destination{safety.Metrics, safety.Metadata, safety.Processes, safety.Connections} {
		if len(destinations[stream]) == 0 {
			return nil, fmt.Errorf("unresolved delivery route %s", stream)
		}
		keys := map[string][]utils.APIKeys{}
		for _, endpoint := range destinations[stream] {
			if err := safety.ValidateEndpoint(endpoint); err != nil {
				return nil, err
			}
			keys[endpoint] = []utils.APIKeys{utils.NewAPIKeys("api_key", apiKey)}
		}
		opts, err := forwarder.NewOptions(p.Config, logger, keys)
		if err != nil {
			return nil, err
		}
		opts.DisableAPIKeyChecking = true
		opts.DeliveryTracker = p.Tracker
		opts.DeliveryContext = ctx
		opts.SetTransport(transport)
		f := forwarder.NewDefaultForwarder(p.Config, logger, opts)
		p.forwarders = append(p.forwarders, f)
		if err := f.Start(); err != nil {
			return nil, err
		}
		byStream[stream] = f
	}
	for stream, prefix := range map[safety.Destination]string{safety.EventPlatform: "software_inventory.forwarder.", safety.NDM: "network_devices.metadata."} {
		endpoints := destinations[stream]
		if len(endpoints) == 0 {
			return nil, fmt.Errorf("unresolved delivery route %s", stream)
		}
		var additional []map[string]any
		for i, endpoint := range endpoints {
			if err := safety.ValidateEndpoint(endpoint); err != nil {
				return nil, err
			}
			u, _ := url.Parse(endpoint)
			if i == 0 {
				set(prefix+"logs_dd_url", u.Hostname()+":443")
			} else {
				additional = append(additional, map[string]any{"host": u.Hostname(), "port": 443, "api_key": apiKey, "is_reliable": true})
			}
		}
		set(prefix+"logs_no_ssl", false)
		set(prefix+"batch_wait", 1.0)
		if len(additional) > 0 {
			encoded, _ := json.Marshal(additional)
			set(prefix+"additional_endpoints", string(encoded))
		}
	}
	events, err := eventplatform.NewIsolatedForwarder(p.Config, Hostname("capture-host"), logscompression.NewComponent(), &secretnoop.SecretNoop{}, transport)
	if err != nil {
		return nil, err
	}
	p.Events = events
	p.Events.Start()
	p.Serializer = serializer.NewSerializer(metadataRouter{Forwarder: byStream[safety.Metrics], metadata: byStream[safety.Metadata]}, nil, compressor, p.Config, logger, "capture-host")
	p.Serializer.RequireCompleteDelivery = true
	syscfg := &sysprobeConfig{BuildableConfig: p.Config.(*isolatedConfig).BuildableConfig}
	p.Submitter, err = runner.NewSubmitter(p.Config, logger, processForwarders{byStream[safety.Processes], byStream[safety.Connections]}, &statsd.NoOpClient{}, "capture-host", syscfg)
	if err != nil {
		return nil, err
	}
	if err := p.Submitter.Start(); err != nil {
		return nil, err
	}
	success = true
	return p, nil
}

func (p *Pipeline) Process(ctx context.Context, at time.Time, body *model.CollectorProc) error {
	return p.Submitter.SubmitForHost(ctx, at, checks.ProcessCheckName, body.HostName, &types.Payload{Message: []model.MessageBody{body}})
}
func (p *Pipeline) Connections(ctx context.Context, at time.Time, body *model.CollectorConnections) error {
	return p.Submitter.SubmitForHost(ctx, at, checks.ConnectionsCheckName, body.HostName, &types.Payload{Message: []model.MessageBody{body}})
}

// Group submits an entire captured collection through one ordinary queue
// entry, retaining the request-ID chunk indices used for ordered delivery.
func (p *Pipeline) Group(ctx context.Context, at time.Time, check, host string, bodies []model.MessageBody) error {
	return p.Submitter.SubmitForHost(ctx, at, check, host, &types.Payload{Message: bodies})
}
func (p *Pipeline) Event(ctx context.Context, eventType string, body []byte, at time.Time) error {
	return p.Events.Send(ctx, message.NewMessage(body, nil, "", at.UnixNano()), eventType)
}
func (p *Pipeline) Wait(ctx context.Context) error {
	if err := p.Tracker.Wait(ctx); err != nil {
		return err
	}
	return p.Events.Tracker.Wait(ctx)
}
func (p *Pipeline) Close() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.Submitter != nil {
		p.Submitter.Stop()
	}
	if p.Events != nil {
		p.Events.Stop()
	}
	for _, f := range p.forwarders {
		f.Stop()
	}
}
