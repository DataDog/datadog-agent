// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpc provides a gRPC client that fits the gRPC server.
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"time"

	pbStream "github.com/pahanini/go-grpc-bidirectional-streaming-example/src/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/grpc/examples/helloworld/helloworld"
	"google.golang.org/grpc/examples/route_guide/routeguide"
)

const (
	defaultDialTimeout = 5 * time.Second
)

// HandleUnary performs a gRPC unary call to SayHello RPC of the greeter service.
func (c *Client) HandleUnary(ctx context.Context, name string) error {
	_, err := c.greeterClient.SayHello(ctx, &pb.HelloRequest{Name: name}, grpc.MaxCallRecvMsgSize(100*1024*1024), grpc.MaxCallSendMsgSize(100*1024*1024))
	return err
}

// HandleStream performs a gRPC stream call to FetchResponse RPC of StreamService service.
func (c *Client) HandleStream(ctx context.Context, numberOfMessages int32) error {
	stream, err := c.streamClient.Max(ctx)
	if err != nil {
		return err
	}

	input := make([]int32, numberOfMessages)
	for i := int32(0); i < numberOfMessages; i++ {
		// The array is zero based, but we want the values to be 1 based.
		input[i] = i + 1
	}

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(input), func(i, j int) { input[i], input[j] = input[j], input[i] })

	var max int32
	var sendErr error
	// A go routine to send input requests
	go func() {
		defer func() { _ = stream.CloseSend() }()
		for _, elem := range input {
			if err := stream.Send(&pbStream.Request{Num: elem}); err != nil {
				sendErr = err
				return
			}
		}
	}()

	var receiveErr error
	// A go routine to receive the requests and save the max number
	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				receiveErr = err
				return
			}
			max = resp.Result
		}
	}()

	<-stream.Context().Done()

	if sendErr != nil {
		return sendErr
	}

	if receiveErr != nil {
		return receiveErr
	}
	if max != numberOfMessages {
		return fmt.Errorf("expected to have %d as max, but instead got %d", numberOfMessages, max)
	}
	return nil
}

// GetFeature activates the GetFeature RPC.
func (c *Client) GetFeature(ctx context.Context, long, lat int32) error {
	_, err := c.routeGuideClient.GetFeature(ctx, &routeguide.Point{
		Latitude:  lat,
		Longitude: long,
	})
	return err
}

// ListFeatures activates the ListFeatures RPC.
func (c *Client) ListFeatures(ctx context.Context, longLo, latLo, longHi, latHi int32) error {
	stream, err := c.routeGuideClient.ListFeatures(ctx, &routeguide.Rectangle{
		Lo: &routeguide.Point{
			Latitude:  latLo,
			Longitude: longLo,
		},
		Hi: &routeguide.Point{
			Latitude:  latHi,
			Longitude: longHi,
		},
	})
	if err != nil {
		return err
	}
	for {
		feature, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fmt.Printf("Feature: name: %q, point:(%v, %v)\n", feature.GetName(),
			feature.GetLocation().GetLatitude(), feature.GetLocation().GetLongitude())
	}

	return nil
}

// Client represents a single gRPC client that fits the gRPC server.
type Client struct {
	conn             *grpc.ClientConn
	greeterClient    pb.GreeterClient
	streamClient     pbStream.MathClient
	routeGuideClient routeguide.RouteGuideClient
}

// Options allows to determine the behavior of the client.
type Options struct {
	// DialTimeout the timeout before giving up on a dialing to the server. Set as 0 for the default (currently 5 seconds).
	DialTimeout time.Duration
	// CustomDialer allows to modify the underlying dialer used by grpc package. Set nil for the default dialer.
	CustomDialer *net.Dialer
}

// NewClient returns a new gRPC client
func NewClient(addr string, options Options, withTLS bool) (Client, error) {
	gRPCOptions := []grpc.DialOption{grpc.WithBlock()} //nolint:staticcheck // TODO (ASC) fix grpc.WithBlock is deprecated
	creds := grpc.WithTransportCredentials(insecure.NewCredentials())
	if withTLS {
		creds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))
	}
	gRPCOptions = append(gRPCOptions, creds)

	if options.CustomDialer != nil {
		gRPCOptions = append(gRPCOptions, grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return options.CustomDialer.DialContext(ctx, "tcp", addr)
		}))
	}

	timeout := defaultDialTimeout
	if options.DialTimeout != 0 {
		timeout = options.DialTimeout
	}
	timedContext, cancel := context.WithTimeout(context.Background(), timeout)
	conn, err := grpc.DialContext(timedContext, addr, gRPCOptions...) //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
	cancel()
	if err != nil {
		return Client{}, err
	}
	return Client{
		conn:             conn,
		greeterClient:    pb.NewGreeterClient(conn),
		streamClient:     pbStream.NewMathClient(conn),
		routeGuideClient: routeguide.NewRouteGuideClient(conn),
	}, nil
}

// Close terminates the client connection.
func (c *Client) Close() {
	c.conn.Close()
}
