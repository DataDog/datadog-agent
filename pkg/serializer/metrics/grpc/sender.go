// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Config carries the per-Sender configuration. Constructed from the
// `serializer_experimental_use_v3_stateful_api.series.grpc.*` config keys
// (contract.md D9). Defaults are applied by the constructor in
// pkg/serializer/metrics.go; this package treats Config as already-validated.
type Config struct {
	Host             string
	Port             int
	APIKey           string
	UseSSL           bool
	UseCompression   bool
	CompressionKind  string // "zstd" | "gzip" | "identity"
	CompressionLevel int

	StreamLifetime    time.Duration
	ConnectionTimeout time.Duration
	DrainTimeout      time.Duration
	MaxInflight       int

	BackoffFactor    float64
	BackoffBase      time.Duration
	BackoffMax       time.Duration
	RecoveryInterval int
}

// Sender manages the gRPC connection to intake's StatefulMetricsService and
// one or more streamWorker lanes. PoC ships with a single lane (N=1); the
// Submit dispatcher hard-codes routing to lane 0. When production
// parallelism becomes a need, the dispatcher grows a Route(*Payload) → laneIdx
// function and Submit fans out accordingly. The wire format does not change.
//
// Compared to pkg/logs/sender/grpc/Sender, this type is significantly
// trimmed: no PipelineComponent interface (no round-robin In() ↔ queue map),
// no pipeline monitor, no auditor sink. The entry point is the explicit
// Submit method.
type Sender struct {
	cfg    Config
	conn   *grpc.ClientConn
	client statefulpb.StatefulMetricsServiceClient

	// PoC: one lane. Slice for future multi-lane parallelism.
	lanes      []*streamWorker
	laneInputs []chan *Payload

	parentCtx context.Context
	cancel    context.CancelFunc
	started   bool

	// One-shot log to confirm series are arriving on the wire path.
	firstSubmitOnce sync.Once
}

// NewSender constructs the Sender, dialing the gRPC connection lazily
// (grpc.NewClient does not block). Returns an error if the connection
// cannot be constructed; if dialing fails later, that's surfaced through
// stream-creation errors and the backoff loop.
func NewSender(cfg Config) (*Sender, error) {
	if cfg.MaxInflight <= 0 {
		return nil, errors.New("stateful metrics sender: MaxInflight must be > 0")
	}
	if cfg.Host == "" || cfg.Port <= 0 {
		return nil, errors.New("stateful metrics sender: Host and Port are required")
	}

	conn, client, err := newGRPCClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("stateful metrics sender: %w", err)
	}

	parentCtx, cancel := context.WithCancel(context.Background())

	// PoC: single lane.
	const numLanes = 1
	s := &Sender{
		cfg:        cfg,
		conn:       conn,
		client:     client,
		lanes:      make([]*streamWorker, 0, numLanes),
		laneInputs: make([]chan *Payload, numLanes),
		parentCtx:  parentCtx,
		cancel:     cancel,
	}

	// Build a compressor that matches the dd-content-encoding header set
	// in newGRPCClient. The header is "identity" when UseCompression=false
	// and cfg.CompressionKind otherwise; the selector takes NoneKind ("none")
	// for the no-compression case (it returns a noop impl). The streamWorker
	// uses this exclusively to compress the rotation snapshot — steady-state
	// batches are compressed by the encoder upstream.
	compressorKind := compressioncommon.NoneKind
	if cfg.UseCompression {
		compressorKind = cfg.CompressionKind
	}
	compressor := selector.NewCompressor(compressorKind, cfg.CompressionLevel)

	for i := 0; i < numLanes; i++ {
		laneID := strconv.Itoa(i)
		inputChan := make(chan *Payload, inputChanBufferSize)
		s.laneInputs[i] = inputChan

		bp := backoff.NewExpBackoffPolicy(
			cfg.BackoffFactor,
			cfg.BackoffBase.Seconds(),
			cfg.BackoffMax.Seconds(),
			cfg.RecoveryInterval,
			true, // RecoveryReset = true; first real ack resets nbErrors.
		)
		worker := newStreamWorker(streamWorkerConfig{
			LaneID:         laneID,
			InputChan:      inputChan,
			ParentContext:  parentCtx,
			Conn:           conn,
			Client:         client,
			StreamLifetime: cfg.StreamLifetime,
			MaxInflight:    cfg.MaxInflight,
			Backoff:        bp,
			Compression:    compressor,
		})
		s.lanes = append(s.lanes, worker)
	}

	return s, nil
}

// inputChanBufferSize bounds the queue between the encoder's Submit calls
// and the streamWorker's supervisor loop. The supervisor reads from this
// channel only when inflight has capacity, providing back-pressure when
// the network is slow.
const inputChanBufferSize = 100

// Start spins up all lane workers. Idempotent.
func (s *Sender) Start() {
	if s.started {
		log.Debug("stateful metrics gRPC sender Start() called but already started")
		return
	}
	s.started = true
	log.Infof("Starting stateful metrics gRPC sender to %s:%d (use_ssl=%t, %d lane(s))",
		s.cfg.Host, s.cfg.Port, s.cfg.UseSSL, len(s.lanes))
	for _, w := range s.lanes {
		w.start()
	}
	log.Info("Stateful metrics gRPC sender started")
}

// Stop halts all lane workers, cancels the parent context (which cascades
// to any in-flight stream contexts), and closes the gRPC connection.
func (s *Sender) Stop() {
	if !s.started {
		return
	}
	log.Info("Stopping stateful metrics gRPC sender")

	for _, w := range s.lanes {
		w.stop()
	}
	for _, c := range s.laneInputs {
		close(c)
	}
	s.cancel()
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			log.Warnf("Error closing stateful metrics gRPC connection: %v", err)
		}
	}
	s.started = false
}

// Submit pushes a payload onto a lane's input queue. PoC: lane 0 always.
// Future multi-lane mode picks a lane via a routing function (see contract
// commitment to defer routing-key choice until SMP-informed).
//
// Returns nil on successful enqueue. Returns an error if the sender is
// stopped (via parent ctx cancel).
func (s *Sender) Submit(payload *Payload) error {
	if payload == nil {
		return nil
	}
	const laneIdx = 0 // PoC: N=1
	// One-shot debug-log of first Submit per process. Helps verify the
	// integration path end-to-end when bringing up the harness.
	s.firstSubmitOnce.Do(func() {
		log.Infof("Stateful metrics gRPC sender received first Submit (encoded=%d B, defines=%d, points=%d)",
			len(payload.Encoded), len(payload.StateChanges), payload.PointCount)
	})
	select {
	case s.laneInputs[laneIdx] <- payload:
		return nil
	case <-s.parentCtx.Done():
		return s.parentCtx.Err()
	}
}

// --------------------------------------------------------------------------
// gRPC client construction
// --------------------------------------------------------------------------

// headerCredentials injects per-RPC metadata (API key, content-encoding,
// version) into every gRPC call on this client.
type headerCredentials struct {
	apiKey          string
	contentEncoding string
}

func (h *headerCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	headers := map[string]string{
		"dd-api-key":          h.apiKey,
		"dd-content-encoding": h.contentEncoding,
	}
	return headers, nil
}

func (h *headerCredentials) RequireTransportSecurity() bool {
	return false // TLS is configured separately via WithTransportCredentials
}

func newGRPCClient(cfg Config) (*grpc.ClientConn, statefulpb.StatefulMetricsServiceClient, error) {
	var opts []grpc.DialOption

	if cfg.UseSSL {
		tlsConfig := &tls.Config{ServerName: cfg.Host}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}))

	opts = append(opts, grpc.WithUserAgent("datadog-agent/"+version.AgentVersion))

	encoding := "identity"
	if cfg.UseCompression {
		encoding = cfg.CompressionKind
	}
	opts = append(opts, grpc.WithPerRPCCredentials(&headerCredentials{
		apiKey:          cfg.APIKey,
		contentEncoding: encoding,
	}))

	opts = append(opts, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))

	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gRPC connection to %s: %w", address, err)
	}
	return conn, statefulpb.NewStatefulMetricsServiceClient(conn), nil
}
