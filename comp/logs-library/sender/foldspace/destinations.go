// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultMaxInflight     = 16
	defaultPipelineDepth   = 8
	defaultStateRequest    = 5 * 1024 * 1024
	defaultShutdownTimeout = 15 * time.Second
)

// SenderSpec is one foldspace sender derived from a logs endpoint.
type SenderSpec struct {
	ID       SenderID
	Address  string
	Class    SenderClass
	APIKey   func() string
	UseTLS   bool
	Endpoint config.Endpoint
}

// DestinationConfig is the driver-facing view of logs_config.foldspace plus
// the HTTP endpoint list.
type DestinationConfig struct {
	Senders           []SenderSpec
	Core              Config
	PipelineDepth     int
	ConnectTimeout    time.Duration
	ShutdownTimeout   time.Duration
	StateRequestBytes int
	DualShip          bool
	SkippedMRF        int
}

// ErrWindowing is returned when max_inflight_payloads is below S × pipeline_depth.
type ErrWindowing struct {
	Senders       int
	PipelineDepth int
	MaxInflight   int
}

func (e ErrWindowing) Error() string {
	return fmt.Sprintf("logs_config.foldspace.max_inflight_payloads (%d) must be >= sender count (%d) × pipeline_depth (%d)",
		e.MaxInflight, e.Senders, e.PipelineDepth)
}

// BuildDestinationConfig maps Endpoints onto foldspace senders.
//
// Input is Endpoints.Endpoints in order: main first, then additional_endpoints
// (and OPW dual-ship extras, which that builder already prepends). MRF entries
// are omitted. foldspace.dd_url overrides only the main host.
func BuildDestinationConfig(cfg pkgconfigmodel.Reader, endpoints *config.Endpoints) (*DestinationConfig, error) {
	if endpoints == nil || len(endpoints.Endpoints) == 0 {
		return nil, fmt.Errorf("foldspace requires at least one HTTP endpoint")
	}

	pipelineDepth := cfg.GetInt("logs_config.foldspace.pipeline_depth")
	if pipelineDepth <= 0 {
		pipelineDepth = defaultPipelineDepth
	}
	maxInflight := cfg.GetInt("logs_config.foldspace.max_inflight_payloads")
	if maxInflight <= 0 {
		maxInflight = defaultMaxInflight
	}
	stateBytes := cfg.GetInt("logs_config.foldspace.state_request_bytes")
	if stateBytes <= 0 {
		stateBytes = defaultStateRequest
	}
	shutdown := cfg.GetDuration("logs_config.foldspace.shutdown_timeout")
	if shutdown <= 0 {
		shutdown = defaultShutdownTimeout
	}
	connectTimeout := time.Duration(cfg.GetInt("logs_config.http_timeout")) * time.Second
	if connectTimeout <= 0 {
		connectTimeout = 10 * time.Second
	}

	mainOverride := strings.TrimSpace(cfg.GetString("logs_config.foldspace.dd_url"))

	var senders []SenderSpec
	var skippedMRF int
	for i, ep := range endpoints.Endpoints {
		if ep.IsMRF {
			skippedMRF++
			log.Infof("foldspace omits MRF endpoint %s:%d", ep.Host, ep.Port)
			continue
		}
		host := ep.Host
		port := ep.Port
		if i == 0 && mainOverride != "" {
			h, p, err := parseHostPort(mainOverride, port)
			if err != nil {
				return nil, fmt.Errorf("logs_config.foldspace.dd_url: %w", err)
			}
			host, port = h, p
		}
		class := Reliable
		if !ep.IsReliable() {
			class = Unreliable
		}
		endpoint := ep
		senders = append(senders, SenderSpec{
			ID:       SenderID(len(senders)),
			Address:  net.JoinHostPort(host, strconv.Itoa(port)),
			Class:    class,
			APIKey:   func() string { return endpoint.GetAPIKey() },
			UseTLS:   endpoint.UseSSL(),
			Endpoint: endpoint,
		})
	}
	if len(senders) == 0 {
		return nil, fmt.Errorf("foldspace requires at least one non-MRF endpoint")
	}
	if len(senders)*pipelineDepth > maxInflight {
		return nil, ErrWindowing{Senders: len(senders), PipelineDepth: pipelineDepth, MaxInflight: maxInflight}
	}

	coreEndpoints := make([]Endpoint, len(senders))
	for i, s := range senders {
		coreEndpoints[i] = Endpoint{Address: s.Address, Class: s.Class}
	}

	compression := Identity
	zstdLevel := 0
	if endpoints.Main.UseCompression {
		if strings.EqualFold(endpoints.Main.CompressionKind, "zstd") {
			compression = Zstd
			zstdLevel = endpoints.Main.CompressionLevel
		} else {
			// gzip is not a library encoding; extras inherit identity and the
			// agent still compresses nothing extra on this path.
			compression = Identity
		}
	}

	batchCapacity := endpoints.BatchMaxSize
	if batchCapacity <= 0 {
		batchCapacity = 100
	}
	maxPayload := endpoints.BatchMaxContentSize
	if maxPayload <= 0 {
		maxPayload = 1024 * 1024
	}

	return &DestinationConfig{
		Senders: senders,
		Core: Config{
			Endpoints:              coreEndpoints,
			MaxInflightPayloads:    maxInflight,
			BatchCapacity:          batchCapacity,
			MaxPayloadBytes:        maxPayload,
			CoalesceThresholdBytes: maxPayload,
			Compression:            compression,
			ZstdLevel:              zstdLevel,
			ReconnectBackoffBase:   time.Duration(cfg.GetFloat64("logs_config.sender_backoff_base") * float64(time.Second)),
			ReconnectBackoffFactor: uint32(cfg.GetInt("logs_config.sender_backoff_factor")),
			ReconnectBackoffCap:    time.Duration(cfg.GetFloat64("logs_config.sender_backoff_max") * float64(time.Second)),
			DrainTimeout:           5 * time.Second,
			StreamLifetime:         endpoints.Main.ConnectionResetInterval,
			FirstPayloadBatchID:    1,
			SnapshotBatchID:        0,
		},
		PipelineDepth:     pipelineDepth,
		ConnectTimeout:    connectTimeout,
		ShutdownTimeout:   shutdown,
		StateRequestBytes: stateBytes,
		DualShip:          cfg.GetBool("logs_config.foldspace.dual_ship"),
		SkippedMRF:        skippedMRF,
	}, nil
}

func parseHostPort(raw string, defaultPort int) (string, int, error) {
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if !strings.Contains(raw, ":") {
		return raw, defaultPort, nil
	}
	host, portStr, err := net.SplitHostPort(raw)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
