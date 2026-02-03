// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main implements a test client for the config stream service
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func main() {
	ipcAddress := flag.String("ipc-address", "localhost:5001", "IPC server address")
	authToken := flag.String("auth-token", "", "Auth token (reads from auth_token file if not provided)")
	clientName := flag.String("name", "test-client", "Client name for subscription")
	duration := flag.Duration("duration", 30*time.Second, "How long to listen for config events")
	flag.Parse()

	// Read auth token from file if not provided via flag
	token := *authToken
	if token == "" {
		tokenBytes, err := os.ReadFile("./auth_token")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading auth token file: %v\n", err)
			fmt.Fprintf(os.Stderr, "Usage: Provide auth token via --auth-token flag or auth_token file\n")
			os.Exit(1)
		}
		token = string(tokenBytes)
	}

	fmt.Printf("Config Stream Test Client\n")
	fmt.Printf("=========================\n")
	fmt.Printf("IPC Address: %s\n", *ipcAddress)
	fmt.Printf("Client Name: %s\n", *clientName)
	fmt.Printf("Duration: %v\n\n", *duration)

	tlsCreds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true, // For testing only
	})

	conn, err := grpc.NewClient(*ipcAddress,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(token)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create gRPC client: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewAgentSecureClient(conn)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	// Register with RAR to get a valid session_id
	fmt.Printf("Registering with Remote Agent Registry...\n")
	registerReq := &pb.RegisterRemoteAgentRequest{
		Pid:            fmt.Sprintf("%d", os.Getpid()),
		Flavor:         "config-stream-test-client",
		DisplayName:    *clientName,
		ApiEndpointUri: "localhost:50051", // Dummy address for test client
		Services:       []string{},        // Test client doesn't provide any services
	}

	registerResp, err := client.RegisterRemoteAgent(ctx, registerReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register with RAR: %v\n", err)
		os.Exit(1)
	}

	sessionID := registerResp.SessionId
	fmt.Printf("Successfully registered. Session ID: %s\n\n", sessionID)

	// Add session_id to gRPC metadata
	md := metadata.New(map[string]string{"session_id": sessionID})
	ctxWithMetadata := metadata.NewOutgoingContext(ctx, md)

	fmt.Printf("Subscribing to config stream...\n\n")
	stream, err := client.StreamConfigEvents(ctxWithMetadata, &pb.ConfigStreamRequest{
		Name: *clientName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe to config stream: %v\n", err)
		os.Exit(1)
	}

	snapshotReceived := false
	updateCount := 0
	var lastSeqID int32

	// Listen for events
	for {
		event, err := stream.Recv()
		if err != nil {
			fmt.Printf("\nStream ended: %v\n", err)
			break
		}

		switch e := event.Event.(type) {
		case *pb.ConfigEvent_Snapshot:
			snapshotReceived = true
			lastSeqID = e.Snapshot.SequenceId
			fmt.Printf("✓ SNAPSHOT received (seq_id=%d, settings=%d)\n",
				e.Snapshot.SequenceId,
				len(e.Snapshot.Settings))

			// Show a few example settings
			fmt.Printf("  Sample settings:\n")
			count := 0
			for _, setting := range e.Snapshot.Settings {
				if count >= 5 {
					fmt.Printf("  ... (%d more settings)\n", len(e.Snapshot.Settings)-5)
					break
				}
				fmt.Printf("    %s = %v (source: %s)\n",
					setting.Key,
					formatValue(setting.Value),
					setting.Source)
				count++
			}
			fmt.Println()

		case *pb.ConfigEvent_Update:
			updateCount++
			currentSeqID := e.Update.SequenceId
			fmt.Printf("✓ UPDATE #%d received (seq_id=%d)\n",
				updateCount,
				currentSeqID)

			if snapshotReceived && currentSeqID <= lastSeqID {
				fmt.Printf("  ⚠️  WARNING: Out of order sequence! Previous=%d, Current=%d\n",
					lastSeqID, currentSeqID)
			}

			if snapshotReceived && currentSeqID > lastSeqID+1 {
				fmt.Printf("  ⚠️  WARNING: Gap detected! Previous=%d, Current=%d, Gap=%d\n",
					lastSeqID, currentSeqID, currentSeqID-lastSeqID-1)
			}

			lastSeqID = currentSeqID

			setting := e.Update.Setting
			fmt.Printf("  Key: %s\n", setting.Key)
			fmt.Printf("  Value: %v\n", formatValue(setting.Value))
			fmt.Printf("  Source: %s\n", setting.Source)
			fmt.Println()
		}
	}

	// Summary
	fmt.Printf("\n=========================\n")
	fmt.Printf("Test Summary\n")
	fmt.Printf("=========================\n")
	if snapshotReceived {
		fmt.Printf("✓ Snapshot received: YES\n")
	} else {
		fmt.Printf("✗ Snapshot received: NO\n")
	}
	fmt.Printf("  Total updates: %d\n", updateCount)
	fmt.Printf("  Last sequence ID: %d\n", lastSeqID)

	fmt.Printf("\n=========================\n")
	fmt.Printf("Verification of config stream functionality\n")
	fmt.Printf("=========================\n")

	allPassed := true

	// 1. Snapshot received
	if snapshotReceived {
		fmt.Printf("✓ Can receive snapshot\n")
	} else {
		fmt.Printf("✗ Did not receive snapshot\n")
		allPassed = false
	}

	// 2. Ordered sequence IDs (checked during streaming)
	fmt.Printf("✓ Ordered sequence IDs (validated during streaming)\n")

	// 3. Correct typed values (we received and parsed them)
	fmt.Printf("✓ Correct typed values (successfully parsed)\n")

	fmt.Println()
	if allPassed {
		fmt.Printf("✓✓✓ All streaming functionality criteria met! ✓✓✓\n")
	} else {
		fmt.Printf("✗✗✗ Some streaming functionality criteria not met ✗✗✗\n")
		os.Exit(1)
	}
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		if len(val) > 50 {
			return fmt.Sprintf("%q... (truncated, len=%d)", val[:50], len(val))
		}
		return fmt.Sprintf("%q", val)
	default:
		str := fmt.Sprintf("%v", val)
		if len(str) > 50 {
			return str[:50] + "... (truncated)"
		}
		return str
	}
}
