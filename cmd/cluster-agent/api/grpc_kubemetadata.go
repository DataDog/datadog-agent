// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package api

import (
	v1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
)

var kubeMetadataServer = v1.NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore())

func (s *serverSecure) StreamKubeMetadata(req *pbgo.KubeMetadataStreamRequest, srv pbgo.AgentSecure_StreamKubeMetadataServer) error {
	return kubeMetadataServer.StreamKubeMetadata(req, srv)
}
