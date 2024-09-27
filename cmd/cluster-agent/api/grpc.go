// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/server"
)

type serverSecure struct {
	pbgo.UnimplementedAgentSecureServer

	taggerServer *taggerserver.Server
}

func (s *serverSecure) TaggerStreamEntities(req *pbgo.StreamTagsRequest, srv pbgo.AgentSecure_TaggerStreamEntitiesServer) error {
	return s.taggerServer.TaggerStreamEntities(req, srv)
}

func (s *serverSecure) TaggerFetchEntity(ctx context.Context, req *pbgo.FetchEntityRequest) (*pbgo.FetchEntityResponse, error) {
	return s.taggerServer.TaggerFetchEntity(ctx, req)
}
