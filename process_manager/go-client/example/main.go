// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pm "github.com/DataDog/agent-process-manager/go-client"
)

func main() {
	// Connect to the daemon
	// Use Unix socket (default): client, err := pm.NewClient()
	// Use TCP (for Docker/remote): client, err := pm.NewClientWithAddress("localhost:50051")
	client, err := pm.NewClientWithAddress("localhost:50051")
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	fmt.Println("=== DataDog Process Manager - Go Client Example ===\n")

	// 1. Create a process
	fmt.Println("1. Creating process...")
	createResp, err := client.CreateProcess(ctx, &pm.CreateRequest{
		Name:    "example-sleep",
		Command: "/bin/sleep",
		Args:    []string{"60"},
		Restart: pm.RestartPolicy_ON_FAILURE,
	})
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}
	processID := createResp.Id
	fmt.Printf("   Created: example-sleep (ID: %s)\n\n", processID)

	// 2. Start the process
	fmt.Println("2. Starting process...")
	if err := client.StartProcess(ctx, processID); err != nil {
		log.Fatalf("Failed to start process: %v", err)
	}
	fmt.Println("   Started successfully\n")

	// Wait a moment for the process to start
	time.Sleep(500 * time.Millisecond)

	// 3. Describe the process
	fmt.Println("3. Getting process details...")
	details, err := client.DescribeProcess(ctx, processID)
	if err != nil {
		log.Fatalf("Failed to describe process: %v", err)
	}
	fmt.Printf("   Name: %s\n", details.Name)
	fmt.Printf("   State: %s\n", details.State)
	fmt.Printf("   PID: %d\n", details.Pid)
	fmt.Printf("   Command: %s %v\n\n", details.Command, details.Args)

	// 4. List all processes
	fmt.Println("4. Listing all processes...")
	processes, err := client.ListProcesses(ctx)
	if err != nil {
		log.Fatalf("Failed to list processes: %v", err)
	}
	fmt.Printf("   Total processes: %d\n", len(processes))
	for _, p := range processes {
		fmt.Printf("   - %s: %s (PID: %d)\n", p.Name, p.State, p.Pid)
	}
	fmt.Println()

	// 5. Check daemon status
	fmt.Println("5. Checking daemon status...")
	status, err := client.GetStatus(ctx)
	if err != nil {
		log.Fatalf("Failed to get status: %v", err)
	}
	fmt.Printf("   Daemon version: %s\n", status.Version)
	fmt.Printf("   Total processes: %d\n\n", status.TotalProcesses)

	// 6. Stop the process
	fmt.Println("6. Stopping process...")
	if err := client.StopProcess(ctx, processID); err != nil {
		log.Fatalf("Failed to stop process: %v", err)
	}
	fmt.Println("   Stopped successfully\n")

	// Wait a moment for the process to stop
	time.Sleep(500 * time.Millisecond)

	// 7. Delete the process
	fmt.Println("7. Deleting process...")
	if err := client.DeleteProcess(ctx, processID, false); err != nil {
		log.Fatalf("Failed to delete process: %v", err)
	}
	fmt.Println("   Deleted successfully\n")

	fmt.Println("=== Example completed successfully ===")
}
