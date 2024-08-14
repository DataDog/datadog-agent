// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopCh := make(chan struct{})
	go handleSignals(stopCh)

	opts := []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
		return net.Dial("tcp", url)
	})}

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})
	opts = append(opts, grpc.WithTransportCredentials(creds))

	conn, err := grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
		ctx,
		fmt.Sprintf(":%v", 5001),
		opts...,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	token := os.Args[1]

	streamCtx, streamCancel := context.WithTimeout(
		metadata.NewOutgoingContext(
			ctx,
			metadata.MD{
				"authorization": []string{
					fmt.Sprintf("Bearer %s", token),
				},
			},
		),
		10*time.Second,
	)

	client := pb.NewAgentSecureClient(conn)

	stream, err := client.AutodiscoveryStreamConfig(streamCtx, nil)
	if err != nil {
		log.Fatal(err)
	}

	for {
		configs, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("%v.AutodiscoveryStreamConfig(_) = _, %v", client, err)
		}
		log.Println(configs)

		select {
		case <-stopCh:
			break
		default:
		}
	}

	cancel()
	streamCancel()
}

// handleSignals handles OS signals, and sends a message on stopCh when an interrupt
// signal is received.
func handleSignals(stopCh chan struct{}) {
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	// Block here until we receive the interrupt signal
	for signo := range signalCh {
		switch signo {
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want dogstatsd to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Printf("Received signal '%s', shutting down...", signo)
			stopCh <- struct{}{}
			return
		}
	}
}
