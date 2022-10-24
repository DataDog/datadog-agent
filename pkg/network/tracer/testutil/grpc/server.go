// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"io"
	"log"
	"net"

	pbStream "github.com/pahanini/go-grpc-bidirectional-streaming-example/src/proto"
	"google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/helloworld/helloworld"
)

// server is used to implement helloworld.GreeterServer.
type server struct {
	pb.UnimplementedGreeterServer
	pbStream.UnimplementedMathServer
}

// SayHello implements helloworld.GreeterServer.
func (server) SayHello(_ context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: %v", in.GetName())
	return &pb.HelloReply{Message: "Hello " + in.GetName()}, nil
}

// FetchResponse implements StreamService.
func (server) Max(srv pbStream.Math_MaxServer) error {
	var max int32
	for {
		select {
		case <-srv.Context().Done():
			return srv.Context().Err()
		default:
		}

		// receive data from stream
		req, err := srv.Recv()
		if err == io.EOF {
			// return will close stream from server side
			log.Println("exit")
			return nil
		}
		if err != nil {
			log.Printf("receive error %v", err)
			continue
		}

		if req.Num <= max {
			continue
		}

		// update max and send it to stream
		max = req.Num
		resp := pbStream.Response{Result: max}
		if err := srv.Send(&resp); err != nil {
			log.Printf("send error %v", err)
		}
	}
}

// Server returns a new instance of the gRPC server.
func Server(addr string) (func(), error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &server{})
	pbStream.RegisterMathServer(s, server{})

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	return func() {
		s.Stop()
	}, nil
}
