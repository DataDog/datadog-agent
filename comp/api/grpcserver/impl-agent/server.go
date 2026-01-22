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