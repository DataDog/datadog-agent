// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// uds-server: Unix Domain Socket server that restarts periodically.
// Simulates an agent that restarts, triggering SIGPIPE in connected clients.
package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

func main() {
	socketPath := os.Getenv("SOCKET_PATH")
	if socketPath == "" {
		socketPath = "/shared/metrics.sock"
	}

	restartInterval := 30
	if envVal := os.Getenv("RESTART_INTERVAL_SECONDS"); envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v > 0 {
			restartInterval = v
		}
	}

	log.Printf("Starting UDS server at %s (will exit in %ds)", socketPath, restartInterval)

	// Remove stale socket file
	os.Remove(socketPath)

	// Create listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	// Make socket world-writable so other containers can connect
	if err := os.Chmod(socketPath, 0777); err != nil {
		log.Printf("Warning: failed to chmod socket: %v", err)
	}

	log.Printf("Listening on %s", socketPath)

	// Set up restart timer
	restartTimer := time.NewTimer(time.Duration(restartInterval) * time.Second)

	// Accept connections in goroutine
	connChan := make(chan net.Conn)
	errChan := make(chan error)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errChan <- err
				return
			}
			connChan <- conn
		}
	}()

	// Handle connections until restart timer fires
	for {
		select {
		case <-restartTimer.C:
			log.Printf("Restart interval reached (%ds), exiting cleanly", restartInterval)
			// Close listener, which will cause Accept to fail
			listener.Close()
			// Give a moment for cleanup
			time.Sleep(100 * time.Millisecond)
			os.Exit(0)

		case conn := <-connChan:
			go handleConnection(conn)

		case err := <-errChan:
			// Accept failed, probably because we closed the listener
			log.Printf("Accept error: %v", err)
			return
		}
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Client connected: %v", conn.RemoteAddr())

	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("Client disconnected: %v", err)
			return
		}
		// Log received data (truncated)
		data := string(buf[:n])
		if len(data) > 50 {
			data = data[:50] + "..."
		}
		fmt.Printf("Received %d bytes: %s\n", n, data)
	}
}
