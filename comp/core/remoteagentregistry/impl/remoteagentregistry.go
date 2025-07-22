// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	raproto "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/proto"
	remoteagentregistryStatus "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/status"
	util "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/util"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the remoteagentregistry component
type Requires struct {
	Config    config.Component
	Lifecycle compdef.Lifecycle
	Telemetry telemetry.Component
}

// Provides defines the output of the remoteagentregistry component
type Provides struct {
	Comp          remoteagentregistry.Component
	FlareProvider flaretypes.Provider
	Status        status.InformationProvider
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) Provides {
	enabled := reqs.Config.GetBool("remote_agent_registry.enabled")
	if !enabled {
		return Provides{}
	}

	ra := newRemoteAgent(reqs)

	return Provides{
		Comp:          ra,
		FlareProvider: flaretypes.NewProvider(ra.fillFlare),
		Status:        status.NewInformationProvider(remoteagentregistryStatus.GetProvider(ra)),
	}
}

func newRemoteAgent(reqs Requires) *remoteAgentRegistry {
	shutdownChan := make(chan struct{})
	comp := &remoteAgentRegistry{
		conf:             reqs.Config,
		agentMap:         make(map[string]*remoteAgentDetails),
		shutdownChan:     shutdownChan,
		telemetry:        reqs.Telemetry,
		configEventsChan: make(chan *settingsUpdates),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			go comp.start()
			return nil
		},
		OnStop: func(context.Context) error {
			shutdownChan <- struct{}{}
			return nil
		},
	})

	return comp
}

// remoteAgentRegistry is the main registry for remote agents. It tracks which remote agents are currently registered, when
// they were last seen, and handles collecting status and flare data from them on request.
type remoteAgentRegistry struct {
	conf             config.Component
	agentMap         map[string]*remoteAgentDetails
	agentMapMu       sync.Mutex
	shutdownChan     chan struct{}
	telemetry        telemetry.Component
	configEventsChan chan *settingsUpdates
}

type settingsUpdates struct {
	setting    string
	newValue   interface{}
	sequenceID uint64
}

// RegisterRemoteAgent registers a remote agent with the registry.
//
// If the remote agent is not present in the registry, it is added. If a remote agent with the same ID is already
// present, the API endpoint and display name are checked: if they are the same, then the "last seen" time of the remote
// agent is updated, otherwise the remote agent is removed and replaced with the incoming one.
func (ra *remoteAgentRegistry) RegisterRemoteAgent(registration *remoteagentregistry.RegistrationData) (uint32, error) {
	recommendedRefreshInterval := uint32(ra.conf.GetDuration("remote_agent_registry.recommended_refresh_interval").Seconds())

	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	// If the remote agent is already registered, then we might be dealing with an update (i.e., periodic check-in) or a
	// brand new remote agent. As the agent ID may very well not be a unique ID every single time (it could always just
	// be `process-agent` or what have you), we differentiate between the two scenarios by checking the human name and
	// API endpoint give to us.
	//
	// If either of them are different, then we remove the old remote agent and add the new one. If they're the same,
	// then we just update the last seen time and move on.
	agentID := util.SanitizeAgentID(registration.AgentID)
	entry, ok := ra.agentMap[agentID]
	if !ok {
		// We've got a brand new remote agent, so add them to the registry.
		//
		// This won't try and connect to the given gRPC endpoint immediately, but will instead surface any errors with
		// connecting when we try to query the remote agent for status or flare data.
		newEntry, err := newRemoteAgentDetails(registration, ra.conf)
		if err != nil {
			return 0, err
		}

		ra.agentMap[agentID] = newEntry

		log.Infof("Remote agent '%s' registered.", agentID)

		return recommendedRefreshInterval, nil
	}

	// We already have an entry for this remote agent, so check if we need to update the gRPC client, and then update
	// the other bits.
	if entry.apiEndpoint != registration.APIEndpoint {
		// The API endpoint has changed, so we need to create a new client and restart the config stream.
		// To do that, we'll just remove the old entry and create a new one.
		entry.configStream.Cancel()
		delete(ra.agentMap, agentID)

		newEntry, err := newRemoteAgentDetails(registration, ra.conf)
		if err != nil {
			return 0, err
		}

		ra.agentMap[agentID] = newEntry
	} else {
		entry.displayName = registration.DisplayName
		entry.lastSeen = time.Now()
	}

	return recommendedRefreshInterval, nil
}

func (ra *remoteAgentRegistry) registerCollector() {
	ra.telemetry.RegisterCollector(newRegistryCollector(&ra.agentMapMu, ra.agentMap, ra.getQueryTimeout()))
}

// Start starts the remote agent registry, which periodically checks for idle remote agents and deregisters them.
func (ra *remoteAgentRegistry) start() {
	remoteAgentIdleTimeout := ra.conf.GetDuration("remote_agent_registry.idle_timeout")
	ra.registerCollector()
	ra.conf.OnUpdate(func(setting string, _ interface{}, newValue interface{}, sequenceID uint64) {
		ra.configEventsChan <- &settingsUpdates{
			setting:    setting,
			newValue:   newValue,
			sequenceID: sequenceID,
		}
	})

	go func() {
		log.Info("Remote Agent registry started.")

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ra.shutdownChan:
				log.Info("Remote Agent registry stopped.")
				return
			case update := <-ra.configEventsChan:
				ra.handleConfigUpdate(update)
			case <-ticker.C:
				ra.agentMapMu.Lock()

				agentsToRemove := make([]string, 0)
				for id, details := range ra.agentMap {
					if time.Since(details.lastSeen) > remoteAgentIdleTimeout {
						agentsToRemove = append(agentsToRemove, id)
					}
				}

				for _, id := range agentsToRemove {
					details, ok := ra.agentMap[id]
					if ok {
						details.configStream.Cancel()

						delete(ra.agentMap, id)
						log.Infof("Remote agent '%s' deregistered after being idle for %s.", id, remoteAgentIdleTimeout)
					}
				}

				ra.agentMapMu.Unlock()
			}
		}
	}()
}

type registryCollector struct {
	agentMapMu   *sync.Mutex
	agentMap     map[string]*remoteAgentDetails
	queryTimeout time.Duration
}

func newRegistryCollector(agentMapMu *sync.Mutex, agentMap map[string]*remoteAgentDetails, queryTimeout time.Duration) prometheus.Collector {
	return &registryCollector{
		agentMapMu:   agentMapMu,
		agentMap:     agentMap,
		queryTimeout: queryTimeout,
	}
}

func (c *registryCollector) Describe(_ chan<- *prometheus.Desc) {
}

func (c *registryCollector) Collect(ch chan<- prometheus.Metric) {
	c.getRegisteredAgentsTelemetry(ch)
}

func (c *registryCollector) getRegisteredAgentsTelemetry(ch chan<- prometheus.Metric) {
	c.agentMapMu.Lock()

	agentsLen := len(c.agentMap)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		c.agentMapMu.Unlock()
		return
	}

	data := make(chan *pb.GetTelemetryResponse, agentsLen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for agentID, details := range c.agentMap {
		go func() {
			resp, err := details.client.GetTelemetry(ctx, &pb.GetTelemetryRequest{}, grpc.WaitForReady(true))
			if err != nil {
				log.Warnf("Failed to query remote agent '%s' for telemetry data: %v", agentID, err)
				return
			}
			if promText, ok := resp.Payload.(*pb.GetTelemetryResponse_PromText); ok {
				collectFromPromText(ch, promText.PromText)
			}
			data <- resp
		}()
	}

	c.agentMapMu.Unlock()

	timeout := time.After(c.queryTimeout)
	responsesRemaining := agentsLen

collect:
	for {
		select {
		case <-data:
			responsesRemaining--
		case <-timeout:
			break collect
		default:
			if responsesRemaining == 0 {
				break collect
			}
		}
	}

}

// Retrieve the telemetry data in exposition format from the remote agent
func collectFromPromText(ch chan<- prometheus.Metric, promText string) {
	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(promText))
	if err != nil {
		log.Warnf("Failed to parse prometheus text: %v", err)
		return
	}
	for _, mf := range metricFamilies {
		help := ""
		if mf.Help != nil {
			help = *mf.Help
		}
		for _, metric := range mf.Metric {
			labelNames := make([]string, 0, len(metric.Label))
			labelValues := make([]string, 0, len(metric.Label))
			for _, label := range metric.Label {
				labelNames = append(labelNames, *label.Name)
				labelValues = append(labelValues, *label.Value)
			}
			switch *mf.Type {
			case dto.MetricType_COUNTER:
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(*mf.Name, help, labelNames, nil),
					prometheus.CounterValue,
					*metric.Counter.Value,
					labelValues...,
				)
			case dto.MetricType_GAUGE:
				ch <- prometheus.MustNewConstMetric(
					prometheus.NewDesc(*mf.Name, help, labelNames, nil),
					prometheus.GaugeValue,
					*metric.Gauge.Value,
					labelValues...,
				)
			}
		}
	}
}

func (ra *remoteAgentRegistry) getQueryTimeout() time.Duration {
	return ra.conf.GetDuration("remote_agent_registry.query_timeout")
}

// GetRegisteredAgents returns the list of registered remote agents.
func (ra *remoteAgentRegistry) GetRegisteredAgents() []*remoteagentregistry.RegisteredAgent {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	agents := make([]*remoteagentregistry.RegisteredAgent, 0, len(ra.agentMap))
	for _, details := range ra.agentMap {
		agents = append(agents, &remoteagentregistry.RegisteredAgent{
			DisplayName:  details.displayName,
			LastSeenUnix: details.lastSeen.Unix(),
		})
	}

	return agents
}

// GetRegisteredAgentStatuses returns the status of all registered remote agents.
func (ra *remoteAgentRegistry) GetRegisteredAgentStatuses() []*remoteagentregistry.StatusData {
	queryTimeout := ra.getQueryTimeout()

	ra.agentMapMu.Lock()

	agentsLen := len(ra.agentMap)
	statusMap := make(map[string]*remoteagentregistry.StatusData, agentsLen)
	agentStatuses := make([]*remoteagentregistry.StatusData, 0, agentsLen)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		ra.agentMapMu.Unlock()
		return agentStatuses
	}

	// We preload the status map with a response that indicates timeout, since we want to ensure there's an entry for
	// every registered remote agent even if we don't get a response back (whether good or bad) from them.
	for agentID, details := range ra.agentMap {
		statusMap[agentID] = &remoteagentregistry.StatusData{
			AgentID:       agentID,
			DisplayName:   details.displayName,
			FailureReason: fmt.Sprintf("Timed out after waiting %s for response.", queryTimeout),
		}
	}

	data := make(chan *remoteagentregistry.StatusData, agentsLen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for agentID, details := range ra.agentMap {
		displayName := details.displayName

		go func() {
			// We push any errors into "failure reason" which ends up getting shown in the status details.
			resp, err := details.client.GetStatusDetails(ctx, &pb.GetStatusDetailsRequest{}, grpc.WaitForReady(true))
			if err != nil {
				data <- &remoteagentregistry.StatusData{
					AgentID:       agentID,
					DisplayName:   displayName,
					FailureReason: fmt.Sprintf("Failed to query for status: %v", err),
				}
				return
			}

			data <- raproto.ProtobufToStatusData(agentID, displayName, resp)
		}()
	}

	ra.agentMapMu.Unlock()

	timeout := time.After(queryTimeout)
	responsesRemaining := agentsLen

collect:
	for {
		select {
		case statusData := <-data:
			statusMap[statusData.AgentID] = statusData
			responsesRemaining--
		case <-timeout:
			break collect
		default:
			if responsesRemaining == 0 {
				break collect
			}
		}
	}

	// Migrate the final status data from the map into our slice, for easier consumption.
	for _, statusData := range statusMap {
		agentStatuses = append(agentStatuses, statusData)
	}

	return agentStatuses
}

func (ra *remoteAgentRegistry) fillFlare(builder flarebuilder.FlareBuilder) error {
	queryTimeout := ra.getQueryTimeout()

	ra.agentMapMu.Lock()

	agentsLen := len(ra.agentMap)
	flareMap := make(map[string]*remoteagentregistry.FlareData, agentsLen)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		ra.agentMapMu.Unlock()
		return nil
	}

	data := make(chan *remoteagentregistry.FlareData, agentsLen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for agentID, details := range ra.agentMap {
		go func() {
			// We push any errors into "failure reason" which ends up getting shown in the status details.
			resp, err := details.client.GetFlareFiles(ctx, &pb.GetFlareFilesRequest{}, grpc.WaitForReady(true))
			if err != nil {
				log.Warnf("Failed to query remote agent '%s' for flare data: %v", agentID, err)
				data <- nil
				return
			}

			data <- raproto.ProtobufToFlareData(agentID, resp)
		}()
	}

	ra.agentMapMu.Unlock()

	timeout := time.After(queryTimeout)
	responsesRemaining := agentsLen

collect:
	for {
		select {
		case flareData := <-data:
			flareMap[flareData.AgentID] = flareData
			responsesRemaining--
		case <-timeout:
			break collect
		default:
			if responsesRemaining == 0 {
				break collect
			}
		}
	}

	// We've collected all the flare data we can, so now we add it to the flare builder.
	for agentID, flareData := range flareMap {
		if flareData == nil {
			continue
		}

		for fileName, fileData := range flareData.Files {
			err := builder.AddFile(fmt.Sprintf("%s/%s", agentID, util.SanitizeFileName(fileName)), fileData)
			if err != nil {
				return fmt.Errorf("failed to add file '%s' from remote agent '%s' to flare: %w", fileName, agentID, err)
			}
		}
	}

	return nil
}

func (ra *remoteAgentRegistry) handleConfigUpdate(update *settingsUpdates) {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	pbValue, err := structpb.NewValue(update.newValue)
	if err != nil {
		log.Warnf("Failed to convert setting '%s' to structpb.Value: %v", update.setting, err)
		return
	}

	source := ra.conf.GetSource(update.setting).String()
	configUpdate := &pb.ConfigUpdate{
		SequenceId: int32(update.sequenceID),
		Setting: &pb.ConfigSetting{
			Key:    update.setting,
			Value:  pbValue,
			Source: source,
		},
	}

	for agentID, details := range ra.agentMap {
		if !details.configStream.TrySendUpdate(configUpdate) {
			log.Warnf("Remote agent '%s' not processing configuration updates in a timely manner. Dropping update.", agentID)
		}
	}
}

func createConfigSnapshot(conf config.Component) (*pb.ConfigEvent, uint64, error) {
	allSettings, sequenceID := conf.AllSettingsWithSequenceID()

	// Note: AllSettings returns a map[string]interface{}. The inner values may be a type that structpb.NewValue is not able to
	// handle (ex: map[string]string), so we perform a hacky operation by marshalling the data into a JSON string first.
	data, err := json.Marshal(allSettings)
	if err != nil {
		return nil, 0, err
	}

	// Unmarshal the data into a map[string]interface{} so that the values have a type that structpb.NewValue can handle.
	var intermediateMap map[string]interface{}
	if err := json.Unmarshal(data, &intermediateMap); err != nil {
		return nil, 0, err
	}

	settings := make([]*pb.ConfigSetting, 0, len(intermediateMap))
	for setting, value := range intermediateMap {
		pbValue, err := structpb.NewValue(value)
		source := conf.GetSource(setting).String()
		if err != nil {
			log.Warnf("Failed to convert setting '%s' to structpb.Value: %v", setting, err)
			continue
		}
		settings = append(settings, &pb.ConfigSetting{
			Source: source,
			Key:    setting,
			Value:  pbValue,
		})
	}

	snapshot := &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: int32(sequenceID),
				Settings:   settings,
			},
		},
	}

	return snapshot, sequenceID, nil
}

func newRemoteAgentClient(registration *remoteagentregistry.RegistrationData) (pb.RemoteAgentClient, error) {
	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	tlsCreds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	conn, err := grpc.NewClient(registration.APIEndpoint,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(registration.AuthToken)),
		// Set on the higher side to account for the fact that flare file data could be larger than the default 4MB limit.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
	)
	if err != nil {
		return nil, err
	}

	return pb.NewRemoteAgentClient(conn), nil
}

type configStream struct {
	ctxCancel     context.CancelFunc
	configUpdates chan *pb.ConfigUpdate
}

func (cs *configStream) Cancel() {
	cs.ctxCancel()
}

func (cs *configStream) TrySendUpdate(update *pb.ConfigUpdate) bool {
	select {
	case cs.configUpdates <- update:
		return true
	default:
		return false
	}
}

func runConfigStream(ctx context.Context, config config.Component, stream pb.RemoteAgent_StreamConfigEventsClient, configUpdates chan *pb.ConfigUpdate) {
	retryInterval := config.GetDuration("remote_agent_registry.config_stream_retry_interval")

outer:
	for {
		lastEventSequenceID := uint64(0)

		// Start by sending an initial snapshot of the current configuration.
		//
		// We do this to ensure that when we restart this outer loop, we always resynchronize the remote agent by
		// providing a complete snapshot of the current configuration. This lets us handle any errors during send
		// by just restarting the outer loop.
		initialSnapshot, sequenceID, err := createConfigSnapshot(config)
		if err != nil {
			log.Errorf("Failed to create initial config snapshot: %v", err)
			time.Sleep(retryInterval)
			continue
		}

		err = stream.Send(initialSnapshot)
		if err != nil {
			log.Errorf("Failed to send initial config snapshot to remote agent: %v", err)
			time.Sleep(retryInterval)
			continue
		}

		lastEventSequenceID = sequenceID

		// Start processing config updates.
		for {
			select {
			case <-ctx.Done():
				return
			case configUpdate := <-configUpdates:
				// If this update is older than the last event we sent, ignore it.
				//
				// If the sequence ID doesn't immediately follow our last event's sequence ID, then we've out of sync and
				// need to restart the outer loop to resynchronize.
				currentSequenceID := uint64(configUpdate.SequenceId)
				if currentSequenceID <= lastEventSequenceID {
					continue
				}

				if currentSequenceID > lastEventSequenceID+1 {
					continue outer
				}

				update := &pb.ConfigEvent{
					Event: &pb.ConfigEvent_Update{
						Update: configUpdate,
					},
				}

				err := stream.Send(update)
				if err != nil {
					log.Errorf("Failed to send config update to remote agent: %v", err)
					continue outer
				}

				lastEventSequenceID = currentSequenceID
			}
		}
	}
}
