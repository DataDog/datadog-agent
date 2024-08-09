// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO Fix revive linter
package start

import (
	"github.com/golang/protobuf/ptypes/empty"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/server"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type serverSecure struct {
	pb.UnimplementedAgentSecureServer
	autoDiscoveryServer *server.Server
}

// AutodiscoveryStreamConfig streams config changes
func (s *serverSecure) AutodiscoveryStreamConfig(_ *empty.Empty, out pb.AgentSecure_AutodiscoveryStreamConfigServer) error {
	return s.autoDiscoveryServer.StreamConfig(out)
}
