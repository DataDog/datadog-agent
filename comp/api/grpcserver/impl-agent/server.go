// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	autodiscoverystream "github.com/DataDog/datadog-agent/comp/core/autodiscovery/stream"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configstreamServer "github.com/DataDog/datadog-agent/comp/core/configstream/server"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerProto "github.com/DataDog/datadog-agent/comp/core/tagger/proto"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilterServer "github.com/DataDog/datadog-agent/comp/core/workloadfilter/server"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	dsdReplay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
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

func (s *serverSecure) RegisterRemoteAgent(_ context.Context, in *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	if s.remoteAgentRegistry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	registration := &remoteagentregistry.RegistrationData{
		AgentPID:         in.Pid,
		AgentFlavor:      in.Flavor,
		AgentDisplayName: in.DisplayName,
		APIEndpointURI:   in.ApiEndpointUri,
		Services:         in.Services,
	}
	sessionID, recommendedRefreshIntervalSecs, err := s.remoteAgentRegistry.RegisterRemoteAgent(registration)
	if err != nil {
		return nil, err
	}

	return &pb.RegisterRemoteAgentResponse{
		RecommendedRefreshIntervalSecs: recommendedRefreshIntervalSecs,
		SessionId:                      sessionID,
	}, nil
}

func (s *serverSecure) RefreshRemoteAgent(_ context.Context, in *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	if s.remoteAgentRegistry == nil {
		return nil, status.Error(codes.Unimplemented, "remote agent registry not enabled")
	}

	found := s.remoteAgentRegistry.RefreshRemoteAgent(in.SessionId)
	if !found {
		return nil, status.Error(codes.NotFound, "no remote agent found with session ID")
	}
	return &pb.RefreshRemoteAgentResponse{}, nil
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

func (s *serverSecure) RemoteQueryExecute(_ context.Context, req *pb.RemoteQueryExecuteRequest) (*pb.RemoteQueryExecuteResponse, error) {
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

	chunkIndex := int32(0)
	result := s.remoteQueries.ExecuteStream(execReq, func(event check.RemoteQueryStreamEvent) error {
		protoEvent, err := remoteQueryStreamEventFromCheckEvent(event)
		if err != nil {
			return err
		}
		err = stream.Send(&pb.RemoteQueryExecuteChunk{Event: protoEvent, ChunkIndex: chunkIndex})
		chunkIndex++
		return err
	})
	if result.Error != nil {
		return remoteQueryExecuteStreamError(result.Error.Code, result.Error.Message, stream)
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: chunkIndex, Final: true})
}

func remoteQueryExecuteRequestFromProto(req *pb.RemoteQueryExecuteRequest) (remotequeriesimpl.RemoteQueryExecuteRequest, error) {
	target := remotequeriesimpl.RemoteQueryExecuteTarget{
		Host:   req.GetTarget().GetHost(),
		Port:   int(req.GetTarget().GetPort()),
		DBName: req.GetTarget().GetDbname(),
	}
	if req.GetOperation() != "copy_stream" {
		return remotequeriesimpl.RemoteQueryExecuteRequest{}, fmt.Errorf("operation must be copy_stream")
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
		out.Event = &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code:       stringFromMetadata(metadata, "code"),
			Message:    stringFromMetadata(metadata, "message"),
			Retryable:  boolFromMetadata(metadata, "retryable"),
			Attributes: stringAttributes(metadata, "code", "message", "retryable", "sequence"),
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
	if v, ok := metadata[key].(bool); ok {
		return v
	}
	return false
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
	if err := stream.Send(&pb.RemoteQueryExecuteChunk{
		ChunkIndex: 0,
		Event: &pb.RemoteQueryExecuteStreamEvent{Event: &pb.RemoteQueryExecuteStreamEvent_Error{Error: &pb.RemoteQueryStreamError{
			Code:    code,
			Message: message,
		}}},
	}); err != nil {
		return err
	}
	return stream.Send(&pb.RemoteQueryExecuteChunk{ChunkIndex: 1, Final: true})
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
