// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	autodiscoverystream "github.com/DataDog/datadog-agent/comp/core/autodiscovery/stream"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configstreamServer "github.com/DataDog/datadog-agent/comp/core/configstream/server"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerProto "github.com/DataDog/datadog-agent/comp/core/tagger/proto"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilterServer "github.com/DataDog/datadog-agent/comp/core/workloadfilter/server"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	pidmap "github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/def"
	dsdReplay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/hosttags"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	rcservicemrf "github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf/def"
	remotequeriesimpl "github.com/DataDog/datadog-agent/comp/remotequeries/impl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type agentServer struct {
	hostname hostnameinterface.Component

	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentSecureServer
	taggerServer         *taggerserver.Server
	tagProcessor         option.Option[tagger.Processor]
	workloadmetaServer   *workloadmetaServer.Server
	workloadfilterServer *workloadfilterServer.Server
	configService        option.Option[rcservice.Component]
	configServiceMRF     option.Option[rcservicemrf.Component]
	dogstatsdServer      dogstatsdServer.Component
	capture              dsdReplay.Component
	pidMap               pidmap.Component
	remoteAgentRegistry  remoteagentregistry.Component
	autodiscovery        autodiscovery.Component
	configComp           config.Component
	configStreamServer   *configstreamServer.Server
	remoteQueries        *remotequeriesimpl.RemoteQueryExecuteService
	healthPlatformStore  healthplatformstore.Component
}

// remoteAgentServer implements the dedicated RemoteAgent gRPC service, which owns the remote agent lifecycle
// (registration and refresh) and the reporting of operational events back to the Core Agent.
type remoteAgentServer struct {
	pb.UnimplementedRemoteAgentServer
	remoteAgentRegistry remoteagentregistry.Component
}

func (s *agentServer) GetHostname(ctx context.Context, _ *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := s.hostname.Get(ctx)
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// AuthFuncOverride implements the `grpc_auth.ServiceAuthFuncOverride` interface which allows
// override of the AuthFunc registered with the unary interceptor.
//
// see: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/auth#ServiceAuthFuncOverride
func (s *agentServer) AuthFuncOverride(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}

func (s *serverSecure) TaggerStreamEntities(req *pb.StreamTagsRequest, srv pb.AgentSecure_TaggerStreamEntitiesServer) error {
	return s.taggerServer.TaggerStreamEntities(req, srv)
}

// TaggerGenerateContainerIDFromOriginInfo generates a container ID from the Origin Info.
// This function takes an Origin Info but only uses the ExternalData part of it, this is done for backward compatibility.
func (s *serverSecure) TaggerGenerateContainerIDFromOriginInfo(ctx context.Context, req *pb.GenerateContainerIDFromOriginInfoRequest) (*pb.GenerateContainerIDFromOriginInfoResponse, error) {
	return s.taggerServer.TaggerGenerateContainerIDFromOriginInfo(ctx, req)
}

func (s *serverSecure) TaggerFetchEntity(ctx context.Context, req *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	return s.taggerServer.TaggerFetchEntity(ctx, req)
}

// DogstatsdCaptureTrigger triggers a dogstatsd traffic capture for the
// duration specified in the request. If a capture is already in progress,
// an error response is sent back.
func (s *serverSecure) DogstatsdCaptureTrigger(_ context.Context, req *pb.CaptureTriggerRequest) (*pb.CaptureTriggerResponse, error) {
	d, err := time.ParseDuration(req.GetDuration())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	p, err := s.capture.StartCapture(req.GetPath(), d, req.GetCompressed())
	if err != nil {
		return &pb.CaptureTriggerResponse{}, err
	}

	return &pb.CaptureTriggerResponse{Path: p}, nil
}

// DogstatsdSetTaggerState allows setting a captured tagger state in the
// Tagger facilities. This endpoint is used when traffic replays are in
// progress. An empty state or nil request will result in the Tagger
// capture state being reset to nil.
func (s *serverSecure) DogstatsdSetTaggerState(_ context.Context, req *pb.TaggerState) (*pb.TaggerStateResponse, error) {
	// Reset and return if no state pushed
	if req == nil || req.State == nil {
		log.Debugf("API: empty request or state")
		s.pidMap.SetPidMap(nil)
		return &pb.TaggerStateResponse{Loaded: false}, nil
	}

	tagProcessor, isSet := s.tagProcessor.Get()
	if !isSet || tagProcessor == nil {
		log.Debug("Tag processor is unavailable. Cannot set tagger state.")
		return &pb.TaggerStateResponse{Loaded: false}, errors.New("tag processor is unavailable")
	}

	state := make([]*taggerTypes.TagInfo, 0, len(req.State))

	// better stores these as the native type
	for id, entity := range req.State {
		entityID, err := taggerProto.Pb2TaggerEntityID(entity.Id)
		if err != nil {
			log.Errorf("Error getting identity ID for %v: %v", id, err)
			continue
		}

		state = append(state, &taggerTypes.TagInfo{
			Source:               "replay",
			EntityID:             *entityID,
			HighCardTags:         entity.HighCardinalityTags,
			OrchestratorCardTags: entity.OrchestratorCardinalityTags,
			LowCardTags:          entity.LowCardinalityTags,
			StandardTags:         entity.StandardTags,
			ExpiryDate:           time.Now().Add(time.Duration(req.Duration) * time.Millisecond * 2),
		})
	}

	tagProcessor.ProcessTagInfo(state)
	s.pidMap.SetPidMap(req.PidMap)

	log.Debugf("API: loaded state successfully")

	return &pb.TaggerStateResponse{Loaded: true}, nil
}

var rcNotInitializedErr = status.Error(codes.Unimplemented, "remote configuration service not initialized")
var mrfRcNotInitializedErr = status.Error(codes.Unimplemented, "MRF remote configuration service not initialized")

func (s *serverSecure) ClientGetConfigs(ctx context.Context, in *pb.ClientGetConfigsRequest) (*pb.ClientGetConfigsResponse, error) {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ClientGetConfigs(ctx, in)
}

func (s *serverSecure) GetConfigState(_ context.Context, _ *emptypb.Empty) (*pb.GetStateConfigResponse, error) {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ConfigGetState()
}

func (s *serverSecure) ClientGetConfigsHA(ctx context.Context, in *pb.ClientGetConfigsRequest) (*pb.ClientGetConfigsResponse, error) {
	rcServiceMRF, isSet := s.configServiceMRF.Get()
	if !isSet || rcServiceMRF == nil {
		log.Debug(mrfRcNotInitializedErr.Error())
		return nil, mrfRcNotInitializedErr
	}
	return rcServiceMRF.ClientGetConfigs(ctx, in)
}

func (s *serverSecure) GetConfigStateHA(_ context.Context, _ *emptypb.Empty) (*pb.GetStateConfigResponse, error) {
	rcServiceMRF, isSet := s.configServiceMRF.Get()
	if !isSet || rcServiceMRF == nil {
		log.Debug(mrfRcNotInitializedErr.Error())
		return nil, mrfRcNotInitializedErr
	}
	return rcServiceMRF.ConfigGetState()
}

func (s *serverSecure) ResetConfigState(_ context.Context, _ *emptypb.Empty) (*pb.ResetStateConfigResponse, error) {
	rcService, isSet := s.configService.Get()

	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return nil, rcNotInitializedErr
	}
	return rcService.ConfigResetState()
}

// WorkloadmetaStreamEntities streams entities from the workloadmeta store applying the given filter
func (s *serverSecure) WorkloadmetaStreamEntities(in *pb.WorkloadmetaStreamRequest, out pb.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	return s.workloadmetaServer.StreamEntities(in, out)
}

// RegisterRemoteAgent is the AgentSecure copy of the remote agent registration RPC.
//
// Deprecated: this RPC has moved to the dedicated RemoteAgent service. It remains here so existing clients keep working
// and can migrate at their own pace; new clients should use RemoteAgent.RegisterRemoteAgent.
func (s *serverSecure) RegisterRemoteAgent(_ context.Context, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	return registerRemoteAgent(s.remoteAgentRegistry, in)
}

// RefreshRemoteAgent is the AgentSecure copy of the remote agent refresh RPC.
//
// Deprecated: this RPC has moved to the dedicated RemoteAgent service. It remains here so existing clients keep working
// and can migrate at their own pace; new clients should use RemoteAgent.RefreshRemoteAgent.
func (s *serverSecure) RefreshRemoteAgent(_ context.Context, in *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	return refreshRemoteAgent(s.remoteAgentRegistry, in)
}

func (s *remoteAgentServer) RegisterRemoteAgent(_ context.Context, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	return registerRemoteAgent(s.remoteAgentRegistry, in)
}

func (s *remoteAgentServer) RefreshRemoteAgent(_ context.Context, in *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	return refreshRemoteAgent(s.remoteAgentRegistry, in)
}

// ReportRemoteAgentEvent routes operational events reported by a remote agent to the remote agent registry.
func (s *remoteAgentServer) ReportRemoteAgentEvent(_ context.Context, in *pb.ReportRemoteAgentEventRequest) (*pb.ReportRemoteAgentEventResponse, error) {
	if s.remoteAgentRegistry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	events := make([]remoteagentregistry.RemoteAgentEvent, 0, len(in.Events))
	for _, pbEvent := range in.Events {
		event := remoteagentregistry.RemoteAgentEvent{Message: pbEvent.Message}
		switch pbEvent.Details.(type) {
		case *pb.Event_InvalidApiKey:
			event.Details = &remoteagentregistry.InvalidAPIKey{}
		}
		events = append(events, event)
	}

	if err := s.remoteAgentRegistry.ReportRemoteAgentEvent(in.SessionId, events); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &pb.ReportRemoteAgentEventResponse{}, nil
}

// registerRemoteAgent is the shared implementation of the RegisterRemoteAgent RPC, used by both the dedicated
// RemoteAgent service and the deprecated AgentSecure copy so the two cannot drift.
func registerRemoteAgent(registry remoteagentregistry.Component, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	if registry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	registration := &remoteagentregistry.RegistrationData{
		AgentPID:         in.Pid,
		AgentFlavor:      in.Flavor,
		AgentDisplayName: in.DisplayName,
		APIEndpointURI:   in.ApiEndpointUri,
		Services:         in.Services,
	}
	sessionID, recommendedRefreshIntervalSecs, err := registry.RegisterRemoteAgent(registration)
	if err != nil {
		return nil, err
	}

	return &pb.RegisterRemoteAgentResponse{
		RecommendedRefreshIntervalSecs: recommendedRefreshIntervalSecs,
		SessionId:                      sessionID,
	}, nil
}

// refreshRemoteAgent is the shared implementation of the RefreshRemoteAgent RPC, used by both the dedicated
// RemoteAgent service and the deprecated AgentSecure copy so the two cannot drift.
func refreshRemoteAgent(registry remoteagentregistry.Component, in *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	if registry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	found := registry.RefreshRemoteAgent(in.SessionId)
	if !found {
		return nil, status.Error(codes.NotFound, "no remote agent found with session ID")
	}
	return &pb.RefreshRemoteAgentResponse{}, nil
}

func (s *serverSecure) validateSessionID(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if s.remoteAgentRegistry == nil {
		return status.Error(codes.Unavailable, "remote agent registry not available")
	}
	if found := s.remoteAgentRegistry.RefreshRemoteAgent(sessionID); !found {
		return status.Error(codes.Unauthenticated, "invalid or expired remote agent session")
	}
	return nil
}

func (s *serverSecure) ReportHealthIssue(_ context.Context, in *pb.ReportHealthIssueRequest) (*emptypb.Empty, error) {
	if err := s.validateSessionID(in.GetRemoteAgentSessionId()); err != nil {
		return nil, err
	}

	issue := in.GetIssue()
	if issue == nil {
		return nil, status.Error(codes.InvalidArgument, "issue cannot be nil")
	}
	if issue.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "issue id cannot be empty")
	}
	if issue.GetIssueName() == "" {
		return nil, status.Error(codes.InvalidArgument, "issue_name cannot be empty")
	}

	if err := s.healthPlatformStore.ReportIssue(issue); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store issue: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *serverSecure) ResolveHealthIssue(_ context.Context, in *pb.ResolveHealthIssueRequest) (*emptypb.Empty, error) {
	if err := s.validateSessionID(in.GetRemoteAgentSessionId()); err != nil {
		return nil, err
	}
	if in.GetIssueId() == "" {
		return nil, status.Error(codes.InvalidArgument, "issue_id cannot be empty")
	}

	s.healthPlatformStore.ResolveIssue(in.GetIssueId())
	return &emptypb.Empty{}, nil
}

func (s *serverSecure) AutodiscoveryStreamConfig(_ *emptypb.Empty, out pb.AgentSecure_AutodiscoveryStreamConfigServer) error {
	return autodiscoverystream.Config(s.autodiscovery, out)
}

func (s *serverSecure) GetHostTags(ctx context.Context, _ *pb.HostTagRequest) (*pb.HostTagReply, error) {
	tags := hosttags.Get(ctx, true, s.configComp)
	return &pb.HostTagReply{System: tags.System, GoogleCloudPlatform: tags.GoogleCloudPlatform}, nil
}

func (s *serverSecure) StreamConfigEvents(in *pb.ConfigStreamRequest, out pb.AgentSecure_StreamConfigEventsServer) error {
	return s.configStreamServer.StreamConfigEvents(in, out)
}

func init() {
	grpclog.SetLoggerV2(grpc.NewLogger())
}

func (s *serverSecure) CreateConfigSubscription(stream pb.AgentSecure_CreateConfigSubscriptionServer) error {
	rcService, isSet := s.configService.Get()
	if !isSet || rcService == nil {
		log.Debug(rcNotInitializedErr.Error())
		return rcNotInitializedErr
	}
	return rcService.CreateConfigSubscription(stream)
}

func (s *serverSecure) WorkloadFilterEvaluate(ctx context.Context, req *pb.WorkloadFilterEvaluateRequest) (*pb.WorkloadFilterEvaluateResponse, error) {
	return s.workloadfilterServer.WorkloadFilterEvaluate(ctx, req)
}

func (s *serverSecure) RemoteQueryExecute(_ context.Context, _ *pb.RemoteQueryExecuteRequest) (*pb.RemoteQueryExecuteResponse, error) {
	return remoteQueryExecuteErrorResponse(remotequeriesimpl.RemoteQueryStatusInvalidRequest, "remote queries require RemoteQueryExecuteStream with operation copy_stream"), nil
}

func (s *serverSecure) RemoteQueryExecuteStream(req *pb.RemoteQueryExecuteRequest, stream pb.AgentSecure_RemoteQueryExecuteStreamServer) error {
	if s.remoteQueries == nil {
		return remoteQueryExecuteStreamError(remotequeriesimpl.RemoteQueryStatusExecutorUnavailable, "remote query executor is unavailable", stream)
	}

	execReq, err := remoteQueryExecuteRequestFromProto(req)
	if err != nil {
		return remoteQueryExecuteStreamError(remotequeriesimpl.RemoteQueryStatusInvalidRequest, err.Error(), stream)
	}

	coalescer := newRemoteQueryIPCStreamCoalescer(stream)
	result := s.remoteQueries.ExecuteStream(execReq, coalescer.Send)
	if result.Error != nil {
		if err := coalescer.Flush(); err != nil {
			return err
		}
		return remoteQueryExecuteStreamErrorAt(result.Error.Code, result.Error.Message, stream, coalescer.NextChunkIndex())
	}
	if err := coalescer.Flush(); err != nil {
		return err
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: coalescer.NextChunkIndex(), Final: true})
}

const remoteQuerySecureIPCDataFlushBytes = 4_000_000

type remoteQueryIPCStreamCoalescer struct {
	stream      pb.AgentSecure_RemoteQueryExecuteStreamServer
	chunkIndex  int32
	data        bytes.Buffer
	dataOffset  uint64
	dataSeq     uint64
	dataStarted bool
	dataChunks  uint64

	start               time.Time
	firstEventAt        time.Time
	firstDataAt         time.Time
	lastDataAt          time.Time
	upstreamDataEvents  uint64
	upstreamDataBytes   uint64
	coalescedDataEvents uint64
	sendCalls           uint64
	sendDuration        time.Duration
	dataSendDuration    time.Duration
	maxSendDuration     time.Duration
	maxDataSendDuration time.Duration
}

func newRemoteQueryIPCStreamCoalescer(stream pb.AgentSecure_RemoteQueryExecuteStreamServer) *remoteQueryIPCStreamCoalescer {
	return &remoteQueryIPCStreamCoalescer{stream: stream, start: time.Now()}
}

func (c *remoteQueryIPCStreamCoalescer) NextChunkIndex() int32 {
	return c.chunkIndex
}

func (c *remoteQueryIPCStreamCoalescer) Send(event check.RemoteQueryStreamEvent) error {
	if c.firstEventAt.IsZero() {
		c.firstEventAt = time.Now()
	}
	protoEvent, err := remoteQueryStreamEventFromCheckEvent(event)
	if err != nil {
		return err
	}
	data := protoEvent.GetData()
	if data == nil {
		if err := c.Flush(); err != nil {
			return err
		}
		c.addTimingAttributes(protoEvent)
		_, err := c.sendProtoEvent(protoEvent)
		return err
	}

	now := time.Now()
	if c.firstDataAt.IsZero() {
		c.firstDataAt = now
	}
	c.lastDataAt = now
	c.upstreamDataEvents++
	c.upstreamDataBytes += uint64(len(data.GetPayload()))

	if c.dataStarted && data.GetOffset() != c.dataOffset+uint64(c.data.Len()) {
		if err := c.Flush(); err != nil {
			return err
		}
	}
	if !c.dataStarted {
		c.dataStarted = true
		c.dataOffset = data.GetOffset()
		c.dataSeq = protoEvent.GetSequence()
	}
	if _, err := c.data.Write(data.GetPayload()); err != nil {
		return err
	}
	for c.data.Len() >= remoteQuerySecureIPCDataFlushBytes {
		if err := c.flushData(remoteQuerySecureIPCDataFlushBytes); err != nil {
			return err
		}
	}
	return nil
}

func (c *remoteQueryIPCStreamCoalescer) Flush() error {
	if !c.dataStarted || c.data.Len() == 0 {
		c.data.Reset()
		c.dataStarted = false
		return nil
	}
	return c.flushData(c.data.Len())
}

func (c *remoteQueryIPCStreamCoalescer) flushData(size int) error {
	payload := append([]byte(nil), c.data.Bytes()[:size]...)
	protoEvent := &pb.RemoteQueryExecuteStreamEvent{
		Sequence: c.dataSeq + c.dataChunks,
		Event: &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{
			Payload: payload,
			Offset:  c.dataOffset,
			Bytes:   uint64(len(payload)),
		}},
	}
	duration, err := c.sendProtoEvent(protoEvent)
	if err != nil {
		return err
	}
	c.coalescedDataEvents++
	c.dataSendDuration += duration
	if duration > c.maxDataSendDuration {
		c.maxDataSendDuration = duration
	}
	remaining := append([]byte(nil), c.data.Bytes()[size:]...)
	c.data.Reset()
	_, _ = c.data.Write(remaining)
	c.dataOffset += uint64(len(payload))
	c.dataChunks++
	if c.data.Len() == 0 {
		c.dataStarted = false
	}
	return nil
}

func (c *remoteQueryIPCStreamCoalescer) sendProtoEvent(event *pb.RemoteQueryExecuteStreamEvent) (time.Duration, error) {
	start := time.Now()
	if err := c.stream.Send(&pb.RemoteQueryExecuteChunk{Event: event, ChunkIndex: c.chunkIndex}); err != nil {
		return 0, err
	}
	duration := time.Since(start)
	c.sendCalls++
	c.sendDuration += duration
	if duration > c.maxSendDuration {
		c.maxSendDuration = duration
	}
	c.chunkIndex++
	return duration, nil
}

func (c *remoteQueryIPCStreamCoalescer) addTimingAttributes(event *pb.RemoteQueryExecuteStreamEvent) {
	final := event.GetFinal()
	if final == nil {
		return
	}
	if final.Attributes == nil {
		final.Attributes = map[string]string{}
	}
	elapsed := time.Since(c.start)
	final.Attributes["agent_coalesce_flush_bytes"] = strconv.Itoa(remoteQuerySecureIPCDataFlushBytes)
	final.Attributes["agent_upstream_data_events"] = strconv.FormatUint(c.upstreamDataEvents, 10)
	final.Attributes["agent_upstream_data_bytes"] = strconv.FormatUint(c.upstreamDataBytes, 10)
	final.Attributes["agent_coalesced_data_events"] = strconv.FormatUint(c.coalescedDataEvents, 10)
	final.Attributes["agent_ipc_send_calls"] = strconv.FormatUint(c.sendCalls, 10)
	final.Attributes["agent_first_event_latency_ms"] = formatDurationMillis(c.firstEventAt.Sub(c.start))
	final.Attributes["agent_first_data_latency_ms"] = formatDurationMillis(c.firstDataAt.Sub(c.start))
	final.Attributes["agent_upstream_data_span_ms"] = formatDurationMillis(c.lastDataAt.Sub(c.firstDataAt))
	final.Attributes["agent_total_stream_ms"] = formatDurationMillis(elapsed)
	final.Attributes["agent_total_stream_mib_per_second"] = formatMiBPerSecond(c.upstreamDataBytes, elapsed)
	final.Attributes["agent_ipc_send_total_ms"] = formatDurationMillis(c.sendDuration)
	final.Attributes["agent_ipc_send_max_ms"] = formatDurationMillis(c.maxSendDuration)
	final.Attributes["agent_ipc_data_send_total_ms"] = formatDurationMillis(c.dataSendDuration)
	final.Attributes["agent_ipc_data_send_max_ms"] = formatDurationMillis(c.maxDataSendDuration)
}

func formatDurationMillis(duration time.Duration) string {
	if duration <= 0 {
		return "0"
	}
	return strconv.FormatFloat(duration.Seconds()*1000, 'f', 3, 64)
}

func formatMiBPerSecond(bytes uint64, duration time.Duration) string {
	if bytes == 0 || duration <= 0 {
		return "0"
	}
	return strconv.FormatFloat((float64(bytes)/1024/1024)/duration.Seconds(), 'f', 3, 64)
}

func remoteQueryExecuteRequestFromProto(req *pb.RemoteQueryExecuteRequest) (remotequeriesimpl.RemoteQueryExecuteRequest, error) {
	target := remotequeriesimpl.RemoteQueryExecuteTarget{
		Host:             req.GetTarget().GetHost(),
		Port:             int(req.GetTarget().GetPort()),
		DBName:           req.GetTarget().GetDbname(),
		DatabaseInstance: req.GetTarget().GetDatabaseInstance(),
	}
	if req.GetOperation() != "copy_stream" {
		return remotequeriesimpl.RemoteQueryExecuteRequest{}, errors.New("operation must be copy_stream")
	}
	return remotequeriesimpl.NewRemoteQueryCopyStreamExecuteRequest(req.GetIntegration(), target, req.GetQuery(), req.GetFormat(), remoteQueryCopyLimitsFromProto(req.GetCopyLimits()))
}

func remoteQueryStreamEventFromCheckEvent(event check.RemoteQueryStreamEvent) (*pb.RemoteQueryExecuteStreamEvent, error) {
	metadata := map[string]interface{}{}
	if strings.TrimSpace(event.MetadataJSON) != "" {
		if err := json.Unmarshal([]byte(event.MetadataJSON), &metadata); err != nil {
			return nil, err
		}
	}
	sequence := uint64FromMetadata(metadata, "sequence")
	out := &pb.RemoteQueryExecuteStreamEvent{Sequence: sequence}
	switch event.Type {
	case "metadata":
		attrs := stringAttributes(metadata, "operation", "integration", "format", "sequence")
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Metadata{Metadata: &pb.RemoteQueryStreamMetadata{
			Operation:   stringFromMetadata(metadata, "operation"),
			Integration: stringFromMetadata(metadata, "integration"),
			Format:      stringFromMetadata(metadata, "format"),
			Attributes:  attrs,
		}}
	case "data":
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Data{Data: &pb.RemoteQueryStreamData{
			Payload: append([]byte(nil), event.Payload...),
			Offset:  uint64FromMetadata(metadata, "offset"),
			Bytes:   uint64FromMetadata(metadata, "bytes"),
		}}
	case "final":
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Final{Final: &pb.RemoteQueryStreamFinal{
			Status:        stringFromMetadata(metadata, "status"),
			BytesEmitted:  uint64FromMetadata(metadata, "bytes_emitted", "bytesEmitted", "bytes"),
			ChunksEmitted: uint64FromMetadata(metadata, "chunks_emitted", "chunksEmitted", "chunks"),
			Attributes:    stringAttributes(metadata, "status", "sequence", "bytes_emitted", "bytesEmitted", "chunks_emitted", "chunksEmitted"),
		}}
	case "error":
		errorMetadata := mapFromMetadata(metadata, "error")
		code := stringFromMetadata(errorMetadata, "code")
		if code == "" {
			code = stringFromMetadata(metadata, "code")
		}
		message := stringFromMetadata(errorMetadata, "message")
		if message == "" {
			message = stringFromMetadata(metadata, "message")
		}
		retryable, hasRetryable := boolValueFromMetadata(errorMetadata, "retryable")
		if !hasRetryable {
			retryable = boolFromMetadata(metadata, "retryable")
		}
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code:       code,
			Message:    message,
			Retryable:  retryable,
			Attributes: stringAttributes(metadata, "code", "message", "retryable", "error", "sequence"),
		}}
	default:
		return nil, errors.New("unknown remote query stream event type")
	}
	return out, nil
}

func stringFromMetadata(metadata map[string]interface{}, key string) string {
	if v, ok := metadata[key].(string); ok {
		return v
	}
	return ""
}

func boolFromMetadata(metadata map[string]interface{}, key string) bool {
	v, _ := boolValueFromMetadata(metadata, key)
	return v
}

func boolValueFromMetadata(metadata map[string]interface{}, key string) (bool, bool) {
	if v, ok := metadata[key].(bool); ok {
		return v, true
	}
	return false, false
}

func mapFromMetadata(metadata map[string]interface{}, key string) map[string]interface{} {
	if v, ok := metadata[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

func uint64FromMetadata(metadata map[string]interface{}, keys ...string) uint64 {
	for _, key := range keys {
		switch v := metadata[key].(type) {
		case float64:
			if v > 0 {
				return uint64(v)
			}
		case int:
			if v > 0 {
				return uint64(v)
			}
		case json.Number:
			if n, err := strconv.ParseUint(string(v), 10, 64); err == nil {
				return n
			}
		case string:
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

func stringAttributes(metadata map[string]interface{}, exclude ...string) map[string]string {
	excluded := make(map[string]struct{}, len(exclude))
	for _, key := range exclude {
		excluded[key] = struct{}{}
	}
	attrs := make(map[string]string)
	for key, value := range metadata {
		if _, ok := excluded[key]; ok {
			continue
		}
		switch v := value.(type) {
		case string:
			attrs[key] = v
		case float64:
			attrs[key] = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			attrs[key] = strconv.FormatBool(v)
		}
	}
	return attrs
}

func remoteQueryExecuteStreamError(code string, message string, stream pb.AgentSecure_RemoteQueryExecuteStreamServer) error {
	return remoteQueryExecuteStreamErrorAt(code, message, stream, 0)
}

func remoteQueryExecuteStreamErrorAt(code string, message string, stream pb.AgentSecure_RemoteQueryExecuteStreamServer, chunkIndex int32) error {
	if err := stream.Send(&pb.RemoteQueryExecuteChunk{
		ChunkIndex: chunkIndex,
		Event: &pb.RemoteQueryExecuteStreamEvent{Event: &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code:    code,
			Message: message,
		}}},
	}); err != nil {
		return err
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: chunkIndex + 1, Final: true})
}

func remoteQueryCopyLimitsFromProto(limits *pb.RemoteQueryExecuteCopyLimits) *remotequeriesimpl.RemoteQueryExecuteCopyLimits {
	if limits == nil {
		return nil
	}
	return &remotequeriesimpl.RemoteQueryExecuteCopyLimits{
		ChunkBytes:  int(limits.GetChunkBytes()),
		MaxBytes:    int(limits.GetMaxBytes()),
		MaxRowBytes: int(limits.GetMaxRowBytes()),
		TimeoutMs:   int(limits.GetTimeoutMs()),
	}
}

func remoteQueryExecuteErrorResponse(code string, message string) *pb.RemoteQueryExecuteResponse {
	return &pb.RemoteQueryExecuteResponse{
		Status: code,
		Error:  &pb.RemoteQueryExecuteError{Code: code, Message: message},
	}
}

type remoteQueryExecuteJSONResponse struct {
	Status    string                   `json:"status"`
	Error     *remoteQueryExecuteError `json:"error,omitempty"`
	Columns   []map[string]interface{} `json:"columns,omitempty"`
	Rows      []map[string]interface{} `json:"rows,omitempty"`
	Truncated bool                     `json:"truncated,omitempty"`
	Stats     map[string]interface{}   `json:"stats,omitempty"`
}

type remoteQueryExecuteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func remoteQueryExecuteResponseFromJSON(responseJSON string) (*pb.RemoteQueryExecuteResponse, error) {
	var payload remoteQueryExecuteJSONResponse
	decoder := json.NewDecoder(strings.NewReader(responseJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, status.Error(codes.Internal, "remote query executor returned invalid JSON")
	}
	if payload.Status == "" {
		return nil, status.Error(codes.Internal, "remote query executor response missing status")
	}

	out := &pb.RemoteQueryExecuteResponse{
		Status:    payload.Status,
		Truncated: payload.Truncated,
	}
	if payload.Error != nil {
		out.Error = &pb.RemoteQueryExecuteError{Code: payload.Error.Code, Message: payload.Error.Message}
	}
	for _, column := range payload.Columns {
		pbColumn, err := structpb.NewStruct(normalizeRemoteQueryStruct(column))
		if err != nil {
			return nil, status.Error(codes.Internal, "remote query executor returned invalid column data")
		}
		out.Columns = append(out.Columns, pbColumn)
	}
	for _, row := range payload.Rows {
		pbRow, err := structpb.NewStruct(normalizeRemoteQueryStruct(row))
		if err != nil {
			return nil, status.Error(codes.Internal, "remote query executor returned invalid row data")
		}
		out.Rows = append(out.Rows, pbRow)
	}
	if payload.Stats != nil {
		stats, err := structpb.NewStruct(normalizeRemoteQueryStruct(payload.Stats))
		if err != nil {
			return nil, status.Error(codes.Internal, "remote query executor returned invalid stats data")
		}
		out.Stats = stats
	}
	return out, nil
}

func normalizeRemoteQueryStruct(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = normalizeRemoteQueryValue(value)
	}
	return out
}

func normalizeRemoteQueryValue(value interface{}) interface{} {
	switch v := value.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
			return f
		}
		return v.String()
	case map[string]interface{}:
		return normalizeRemoteQueryStruct(v)
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = normalizeRemoteQueryValue(item)
		}
		return out
	default:
		return v
	}
}
