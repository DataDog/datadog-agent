// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// victim-app-c: Go application that uses a C library for metrics reporting.
// Demonstrates SIGPIPE crash when the UDS server (agent) restarts.
//
// The C library spawns a background thread at library load time that
// is NOT managed by Go's runtime, so SIGPIPE will terminate the process.
package main

/*
#cgo CFLAGS: -I.
#cgo LDFLAGS: -L. -lmetrics -lpthread

#include "metrics.h"
*/
import "C"

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	socketPath := os.Getenv("SOCKET_PATH")
	if socketPath == "" {
		socketPath = "/shared/metrics.sock"
	}

	intervalMs := 1000
	if envVal := os.Getenv("WRITE_INTERVAL_MS"); envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v > 0 {
			intervalMs = v
		}
	}

	log.Printf("Starting victim-app-c, writing metrics to %s every %dms", socketPath, intervalMs)

	// Set up signal handling for graceful shutdown (but NOT SIGPIPE)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Wait for socket to be available
	log.Printf("Waiting for socket at %s...", socketPath)
	for {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Printf("Socket found, initializing connection")

	// Initialize connection via C library
	cSocketPath := C.CString(socketPath)
	result := C.init_metrics(cSocketPath)
	if result != 0 {
		log.Printf("Failed to initialize metrics connection, retrying...")
		time.Sleep(time.Second)
		result = C.init_metrics(cSocketPath)
		if result != 0 {
			log.Fatalf("Failed to initialize metrics connection after retry")
		}
	}

	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()
	defer C.close_metrics()

	writeCount := 0
	for {
		select {
		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down gracefully", sig)
			return

		case <-ticker.C:
			// Call C function to write metrics
			// This will crash with SIGPIPE when the socket is closed
			result := C.write_metrics()
			writeCount++

			if result == 0 {
				if writeCount%10 == 0 {
					log.Printf("Wrote %d metrics successfully", writeCount)
				}
			} else {
				// If we get here, the write failed but SIGPIPE didn't kill us
				log.Printf("write_metrics returned error: %d (write #%d), reconnecting...", result, writeCount)
				C.close_metrics()
				time.Sleep(time.Second)
				result = C.init_metrics(cSocketPath)
				if result != 0 {
					log.Printf("Failed to reconnect, will retry next interval")
				} else {
					log.Printf("Reconnected successfully")
				}
			}
		}
	}
}
