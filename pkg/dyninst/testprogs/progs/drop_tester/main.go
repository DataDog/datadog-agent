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
//	fn            one of "bytes", "bytes_return", "chain", "chain_return",
//	              "string_chain"
//	size          payload byte length (entry side, or chain length for "chain")
//	return_size   result byte length (only used by "bytes_return")
//	entry_nodes   chain length on the entry side (only used by "chain_return")
//	return_nodes  chain length on the return side (only used by "chain_return")
//	nodes         chain length (only used by "string_chain")
//	str_len       per-node string length in bytes (only used by "string_chain")
//	iter          number of back-to-back invocations (default 1)
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
	mux.HandleFunc("/chain_return", handleChainReturn)
	mux.HandleFunc("/string_chain", handleStringChain)
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

// StringNode is a linked list whose nodes each carry a string. Used by
// the fragment-cap test (variant_C in TestFirstFlushFailDropNotification)
// to drive enough captured bytes to exceed MAX_CONTINUATION_FRAGMENTS *
// SCRATCH_BUF_LEN. Strings go through chased_slices (cap 128) rather
// than the chased-pointers trie (cap 1024), so a chain of strings can
// produce arbitrarily many bytes without exhausting the trie. A plain
// chain of fixed-size Nodes can't reach the cap because each Node
// consumes a trie slot.
type StringNode struct {
	Value string
	Next  *StringNode
}

// CaptureStringChain is the probe target. A single probe captures
// head + the chain reachable from it; each node's string is collected
// as a separate slice item. Used to push past the per-invocation
// fragment cap.
//
//go:noinline
func CaptureStringChain(head *StringNode) {
	// Walk the chain so the compiler can't eliminate the head field
	// or any of the strings.
	n := 0
	totalLen := 0
	for cur := head; cur != nil; cur = cur.Next {
		n++
		totalLen += len(cur.Value)
	}
	sinkInt = sinkInt ^ n ^ totalLen
}

func buildStringChain(n, strLen int) *StringNode {
	if n <= 0 {
		return nil
	}
	// Each node gets its own backing string allocation. dyninst
	// dedups chased pointers via the chased_pointers_trie /
	// chased_slices set keyed on string-data address; nodes sharing
	// a backing buffer would only get serialized once. Distinct
	// allocations force one capture per node.
	makeValue := func(seed int) string {
		buf := make([]byte, strLen)
		for i := range buf {
			buf[i] = byte((i + seed) % 251)
		}
		return string(buf)
	}
	head := &StringNode{Value: makeValue(0)}
	cur := head
	for i := 1; i < n; i++ {
		cur.Next = &StringNode{Value: makeValue(i)}
		cur = cur.Next
	}
	return head
}

// CaptureChainReturn is a paired (entry + return) probe target. The
// entry probe captures the entry-side chain via head; the return probe
// captures the return-side chain via the returned *Node. Independent
// caller-controlled chain lengths let tests force drops on either side
// independently. Used by the deterministic first-flush-fails test to
// build precise saturation scenarios where the entry-side capture
// produces a single small ringbuf record but the return-side capture
// requires continuation flushes.
//
//go:noinline
func CaptureChainReturn(head *Node, returnLen int) (*Node, error) {
	result := buildChain(returnLen)
	// Walk both chains so the compiler can't eliminate either.
	n := 0
	for cur := head; cur != nil; cur = cur.Next {
		n++
	}
	for cur := result; cur != nil; cur = cur.Next {
		n++
	}
	sinkInt = sinkInt ^ n
	return result, nil
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

func handleChainReturn(w http.ResponseWriter, r *http.Request) {
	entryNodes := parseIntQuery(r, "entry_nodes", 0)
	returnNodes := parseIntQuery(r, "return_nodes", 0)
	iter := parseIntQuery(r, "iter", 1)
	entryChain := buildChain(entryNodes)
	for i := 0; i < iter; i++ {
		_, _ = CaptureChainReturn(entryChain, returnNodes)
	}
	w.WriteHeader(http.StatusOK)
}

func handleStringChain(w http.ResponseWriter, r *http.Request) {
	nodes := parseIntQuery(r, "nodes", 0)
	strLen := parseIntQuery(r, "str_len", 0)
	iter := parseIntQuery(r, "iter", 1)
	chain := buildStringChain(nodes, strLen)
	for i := 0; i < iter; i++ {
		CaptureStringChain(chain)
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
