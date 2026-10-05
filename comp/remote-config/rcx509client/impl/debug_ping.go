// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteconfigv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const debugServicePingURI = "rc.x509.magic_tunnel.remote_config.v1.DebugService/Ping"

func newDebugPingHandler(logger log.Component) func(uint64, []byte) ([]byte, error) {
	return func(correlationID uint64, payload []byte) ([]byte, error) {
		var request remoteconfigv1.PingRequest
		if err := proto.Unmarshal(payload, &request); err != nil {
			return nil, fmt.Errorf("decode x509 debug ping request: %w", err)
		}

		response, err := proto.Marshal(&remoteconfigv1.PingResponse{Now: timestamppb.Now()})
		if err != nil {
			return nil, fmt.Errorf("encode x509 debug ping response: %w", err)
		}

		// Do not log the request: its reason and opaque payload are controlled by
		// the backend. The correlation ID is sufficient to match this response.
		logger.Infof("remote config x509 debug ping handled (correlation_id=%d)", correlationID)
		return response, nil
	}
}
