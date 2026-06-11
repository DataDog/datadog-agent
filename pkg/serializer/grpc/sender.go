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
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Config is the per-Sender configuration. The destination (BaseURL), API key,
// and compressor are derived from the agent's standard config (DomainResolver +
// serializer compressor) by the caller; only the gRPC tuning knobs come from the
// grpc.* keys. Treated as already-validated.
type Config struct {
	// BaseURL is the intake base URL (the resolver's base domain, e.g.
	// "https://api.datadoghq.com"). NewSender parses it into a gRPC dial target
	// (host:port + TLS); a missing scheme is treated as https, a missing port
	// as 443.
	BaseURL string
	// APIKey returns the current API key, called per-RPC so key rotation is
	// picked up without reconnecting. Must be non-nil.
	APIKey func() string

	// Compression is the serializer's payload compressor, reused so the
	// rotation snapshot uses the same codec as payloads (the codec the stream's
	// dd-content-encoding header, from ContentEncoding(), declares). Non-nil.
	Compression compression.Compressor

	StreamLifetime    time.Duration
	ConnectionTimeout time.Duration
	DrainTimeout      time.Duration
	MaxInflight       int

	BackoffFactor    float64
	BackoffBase      time.Duration
	BackoffMax       time.Duration
	RecoveryInterval int
}

// errSenderStopped is returned by Submit once the worker is stopped.
var errSenderStopped = errors.New("stateful metrics sender: stopped")

// Sender is one stateful transport to one destination: a single gRPC connection
// to intake's StatefulMetricsService driving a single streamWorker. It
// implements PayloadSink. Multiplicity lives above it — a Fanout duplicates a
// Payload across destinations; parallelism uses multiple Senders, each with its
// own dictionary (a dictionary cannot be sharded across streams).
type Sender struct {
	cfg     Config
	address string // gRPC dial target parsed from cfg.BaseURL, for logs/telemetry
	conn    *grpc.ClientConn
	client  statefulpb.StatefulMetricsServiceClient

	worker *streamWorker

	cancel  context.CancelFunc
	started bool
}

// NewSender constructs the Sender, dialing the gRPC connection lazily
// (grpc.NewClient does not block). Returns an error if the connection
// cannot be constructed; if dialing fails later, that's surfaced through
// stream-creation errors and the backoff loop.
func NewSender(cfg Config) (*Sender, error) {
	if cfg.MaxInflight <= 0 {
		return nil, errors.New("stateful metrics sender: MaxInflight must be > 0")
	}
	if cfg.APIKey == nil {
		return nil, errors.New("stateful metrics sender: APIKey provider is required")
	}
	if cfg.Compression == nil {
		return nil, errors.New("stateful metrics sender: Compression is required")
	}

	address, useTLS, serverName, err := dialTargetFromBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("stateful metrics sender: %w", err)
	}

	conn, client, err := newGRPCClient(address, useTLS, serverName, cfg.APIKey, cfg.Compression.ContentEncoding())
	if err != nil {
		return nil, fmt.Errorf("stateful metrics sender: %w", err)
	}

	parentCtx, cancel := context.WithCancel(context.Background())

	s := &Sender{
		cfg:     cfg,
		address: address,
		conn:    conn,
		client:  client,
		cancel:  cancel,
	}

	bp := backoff.NewExpBackoffPolicy(
		cfg.BackoffFactor,
		cfg.BackoffBase.Seconds(),
		cfg.BackoffMax.Seconds(),
		cfg.RecoveryInterval,
		true, // RecoveryReset = true; first real ack resets nbErrors.
	)
	s.worker = newStreamWorker(streamWorkerConfig{
		LaneID:         address, // identifies the worker's destination in logs/telemetry
		ParentContext:  parentCtx,
		Conn:           conn,
		Client:         client,
		StreamLifetime: cfg.StreamLifetime,
		MaxInflight:    cfg.MaxInflight,
		Backoff:        bp,
		Compression:    cfg.Compression,
	})

	return s, nil
}

// inputChanBufferSize bounds the queue between Submit calls and the
// streamWorker's supervisor loop. The supervisor reads from this channel only
// when inflight has capacity, providing back-pressure when the network is slow.
const inputChanBufferSize = 100

// Start spins up the worker. Idempotent.
func (s *Sender) Start() {
	if s.started {
		log.Debug("stateful metrics gRPC sender Start() called but already started")
		return
	}
	s.started = true
	log.Infof("Starting stateful metrics gRPC sender to %s", s.address)
	s.worker.start()
	log.Info("Stateful metrics gRPC sender started")
}

// Stop halts the worker, cancels the parent context (which cascades to any
// in-flight stream contexts), and closes the gRPC connection.
func (s *Sender) Stop() {
	if !s.started {
		return
	}
	log.Info("Stopping stateful metrics gRPC sender")

	s.worker.stop()
	s.cancel()
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			log.Warnf("Error closing stateful metrics gRPC connection: %v", err)
		}
	}
	s.started = false
}

// Submit hands a payload to the worker (PayloadSink). Blocks when the input
// channel is full (back-pressure); returns errSenderStopped once the worker is
// stopped, so a Submit racing with Stop doesn't block forever.
func (s *Sender) Submit(payload *Payload) error {
	if payload == nil {
		return nil
	}
	select {
	case s.worker.inputChan <- payload:
		return nil
	case <-s.worker.stopChan:
		return errSenderStopped
	}
}

// --------------------------------------------------------------------------
// gRPC client construction
// --------------------------------------------------------------------------

// headerCredentials injects per-RPC metadata (API key, content-encoding) into
// every gRPC call on this client. apiKey is a provider, called on every RPC, so
// API-key rotation on the resolver is reflected without reconnecting.
type headerCredentials struct {
	apiKey          func() string
	contentEncoding string
}

func (h *headerCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	headers := map[string]string{
		"dd-api-key":          h.apiKey(),
		"dd-content-encoding": h.contentEncoding,
	}
	return headers, nil
}

func (h *headerCredentials) RequireTransportSecurity() bool {
	return false // TLS is configured separately via WithTransportCredentials
}

func newGRPCClient(address string, useTLS bool, serverName string, apiKey func() string, contentEncoding string) (*grpc.ClientConn, statefulpb.StatefulMetricsServiceClient, error) {
	var opts []grpc.DialOption

	if useTLS {
		tlsConfig := &tls.Config{ServerName: serverName}
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

	opts = append(opts, grpc.WithPerRPCCredentials(&headerCredentials{
		apiKey:          apiKey,
		contentEncoding: contentEncoding,
	}))

	opts = append(opts, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))

	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gRPC connection to %s: %w", address, err)
	}
	return conn, statefulpb.NewStatefulMetricsServiceClient(conn), nil
}

// dialTargetFromBaseURL converts the agent's configured intake base URL (e.g.
// "https://api.datadoghq.com", or a bare host) into a gRPC dial target
// "host:port", whether to use TLS, and the TLS server name. gRPC's NewClient
// does not accept an https:// URL (https is not a gRPC resolver scheme), so the
// URL must be reduced to host:port. A missing scheme is treated as https; a
// missing port defaults to 443 (80 without TLS).
func dialTargetFromBaseURL(baseURL string) (address string, useTLS bool, serverName string, err error) {
	if baseURL == "" {
		return "", false, "", errors.New("base URL is empty")
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}
	u, perr := url.Parse(baseURL)
	if perr != nil {
		return "", false, "", fmt.Errorf("parsing base URL %q: %w", baseURL, perr)
	}
	host := u.Hostname()
	if host == "" {
		return "", false, "", fmt.Errorf("base URL %q has no host", baseURL)
	}
	useTLS = u.Scheme != "http"
	port := u.Port()
	if port == "" {
		if useTLS {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port), useTLS, host, nil
}
