// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command drop_tester is an HTTP-driven test program for exercising the
// dyninst ringbuf-drop / side-channel pipeline. Each HTTP request selects
// a probed function and the size of the data it captures, letting the
// integration test drive specific drop scenarios by sending tailored
// requests against a shrunken ringbuf.
//
// URL schema: GET /<fn>?size=<n>&return_size=<n>&iter=<n>
//
//	fn           one of "bytes", "bytes_return", "chain"
//	size         payload byte length (entry side, or chain length for "chain")
//	return_size  result byte length (only used by "bytes_return")
//	iter         number of back-to-back invocations (default 1)
//
// On startup the program prints "Listening on port %d" on stdout so the
// harness can discover the port.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	defer log.Println("drop_tester: stopping")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("Listening on port %d\n", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/bytes", handleBytes)
	mux.HandleFunc("/bytes_return", handleBytesReturn)
	mux.HandleFunc("/chain", handleChain)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	s := &http.Server{Handler: mux, ReadTimeout: 10 * time.Second}
	go func() { _ = s.Serve(ln) }()

	<-ctx.Done()
	log.Println("drop_tester: shutting down HTTP server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.Shutdown(shutdownCtx)
}

func parseIntQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// ----------------------------------------------------------------------
// Instrumented functions.
//
// Each is marked //go:noinline so the dyninst probe can attach reliably
// and both entry and return probes observe the full parameter / return
// sets. The bodies are deliberately trivial: the goal is to expose a
// specific, predictable set of parameters to the probe, sized by the
// caller.
// ----------------------------------------------------------------------

// CaptureBytes is an entry-only probe target. A probe captures the
// payload + tag arguments; request size controls len(payload).
//
//go:noinline
func CaptureBytes(payload []byte, tag string) {
	// sinkByte prevents the compiler from optimizing payload away.
	sinkByte = sinkByte ^ byte(len(payload)) ^ byte(len(tag))
}

// CaptureBytesReturn is a paired (entry + return) probe target. Both
// entry and return probes capture arguments / results. The caller
// controls payload size (entry side) and result size (return side)
// independently so the test can force drops on a specific side.
//
//go:noinline
func CaptureBytesReturn(payload []byte, tag string) ([]byte, error) {
	result := make([]byte, returnSizeFromCaller)
	for i := range result {
		result[i] = byte(i)
	}
	// Use tag so it's not deadcode-eliminated.
	sinkByte = sinkByte ^ byte(len(payload)) ^ byte(len(tag)) ^ byte(len(result))
	return result, nil
}

// Node is a linked-list node whose payload dominates memory so that
// pointer chasing over a Node chain crosses the scratch buffer
// boundary reasonably quickly.
type Node struct {
	Value [64]byte
	Next  *Node
}

// CaptureChain is a pointer-chasing probe target. A probe captures
// head + the chain it points to. Caller-controlled chain length drives
// how much pointer-chased data the probe collects.
//
//go:noinline
func CaptureChain(head *Node) {
	// Walk the chain so the compiler can't eliminate the head field.
	n := 0
	for cur := head; cur != nil; cur = cur.Next {
		n++
	}
	sinkInt = sinkInt ^ n
}

// sinkByte / sinkInt exist to keep the instrumented functions from
// being folded or inlined by the compiler. Their values are
// meaningless; the point is that the instrumented functions touch
// package-level state so the optimizer can't elide them.
var (
	sinkByte byte
	sinkInt  int
	// returnSizeFromCaller is set by handleBytesReturn before calling
	// CaptureBytesReturn. Using a package global rather than a
	// parameter keeps the probe signature predictable (the probe
	// captures `payload` and `tag`, not a dynamic size hint).
	returnSizeFromCaller int
	returnSizeMu         sync.Mutex
)

// ----------------------------------------------------------------------
// HTTP handlers: each parses the request, runs the instrumented
// function `iter` times, and returns 200 OK.
// ----------------------------------------------------------------------

func handleBytes(w http.ResponseWriter, r *http.Request) {
	size := parseIntQuery(r, "size", 0)
	iter := parseIntQuery(r, "iter", 1)
	payload := make([]byte, size)
	for i := 0; i < iter; i++ {
		CaptureBytes(payload, "tag")
	}
	w.WriteHeader(http.StatusOK)
}

func handleBytesReturn(w http.ResponseWriter, r *http.Request) {
	size := parseIntQuery(r, "size", 0)
	returnSize := parseIntQuery(r, "return_size", 0)
	iter := parseIntQuery(r, "iter", 1)
	payload := make([]byte, size)

	// Serialize the global so concurrent requests don't race on it.
	// The instrumented function reads returnSizeFromCaller at call time;
	// holding the lock across Iter calls keeps the value stable for the
	// duration of this request.
	returnSizeMu.Lock()
	defer returnSizeMu.Unlock()
	returnSizeFromCaller = returnSize

	for i := 0; i < iter; i++ {
		_, _ = CaptureBytesReturn(payload, "tag")
	}
	w.WriteHeader(http.StatusOK)
}

func handleChain(w http.ResponseWriter, r *http.Request) {
	length := parseIntQuery(r, "size", 0)
	iter := parseIntQuery(r, "iter", 1)
	chain := buildChain(length)
	for i := 0; i < iter; i++ {
		CaptureChain(chain)
	}
	w.WriteHeader(http.StatusOK)
}

func buildChain(n int) *Node {
	if n <= 0 {
		return nil
	}
	head := &Node{}
	cur := head
	for i := 1; i < n; i++ {
		cur.Next = &Node{}
		cur = cur.Next
	}
	return head
}
