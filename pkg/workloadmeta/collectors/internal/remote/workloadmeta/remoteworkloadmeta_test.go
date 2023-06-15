package remoteworkloadmeta

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type serverSecure struct {
	pbgo.UnimplementedAgentSecureServer
	workloadmetaServer *server.Server
}

func (*serverSecure) TaggerStreamEntities(*pbgo.StreamTagsRequest, pbgo.AgentSecure_TaggerStreamEntitiesServer) error {
	return status.Errorf(codes.Unimplemented, "method TaggerStreamEntities not implemented")
}
func (*serverSecure) TaggerFetchEntity(context.Context, *pbgo.FetchEntityRequest) (*pbgo.FetchEntityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method TaggerFetchEntity not implemented")
}
func (*serverSecure) DogstatsdCaptureTrigger(context.Context, *pbgo.CaptureTriggerRequest) (*pbgo.CaptureTriggerResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DogstatsdCaptureTrigger not implemented")
}
func (*serverSecure) DogstatsdSetTaggerState(context.Context, *pbgo.TaggerState) (*pbgo.TaggerStateResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DogstatsdSetTaggerState not implemented")
}
func (*serverSecure) ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ClientGetConfigs not implemented")
}
func (*serverSecure) GetConfigState(context.Context, *emptypb.Empty) (*pbgo.GetStateConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetConfigState not implemented")
}
func (s *serverSecure) WorkloadmetaStreamEntities(in *pbgo.WorkloadmetaStreamRequest, out pbgo.AgentSecure_WorkloadmetaStreamEntitiesServer) error {
	return s.workloadmetaServer.StreamEntities(in, out)
}

// func TestCollection(t *testing.T) {
// 	mockServerStore := workloadmeta.NewMockStore()
// 	server := &serverSecure{workloadmetaServer: server.NewServer(mockServerStore)}

// 	// gRPC server
// 	grpcServer := grpc.NewServer()
// 	pbgo.RegisterAgentSecureServer(grpcServer, server)

// 	lis, err := net.Listen("tcp", ":0")
// 	assert.NoError(t, err)

// 	go func() {
// 		err := grpcServer.Serve(lis)
// 		if err != nil {
// 			panic(err)
// 		}
// 	}()

// 	_, portStr, err := net.SplitHostPort(lis.Addr().String())
// 	assert.NoError(t, err)
// 	port, err := strconv.Atoi(portStr)
// 	assert.NoError(t, err)

// 	collector := &remote.GenericCollector{
// 		NewClient:       NewAgentSecureClient,
// 		ResponseHandler: handleWorkloadmetaStreamResponse,
// 		Port:            port,
// 	}

// 	mockClientStore := workloadmeta.NewMockStore()
// 	err = collector.Start(context.TODO(), mockClientStore)
// 	assert.NoError(t, err)

// 	expectedContainer := &workloadmeta.Container{
// 		EntityID: workloadmeta.EntityID{
// 			Kind: workloadmeta.KindContainer,
// 			ID:   "cid",
// 		},
// 	}

// 	mockServerStore.Notify(
// 		[]workloadmeta.CollectorEvent{
// 			workloadmeta.CollectorEvent{
// 				Type:   workloadmeta.EventTypeSet,
// 				Source: workloadmeta.SourceAll,
// 				Entity: expectedContainer,
// 			},
// 		},
// 	)
// 	time.Sleep(15 * time.Second)
// 	container, err := mockClientStore.GetContainer("cid")
// 	assert.NoError(t, err)
// 	assert.Equal(t, container, expectedContainer)
// }
