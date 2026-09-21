// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/eksidentity"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

// resolveEKSClusterIdentity is injectable for tests.
var resolveEKSClusterIdentity = eksidentity.Resolve

// getEKSClusterIdentity returns the authoritative EKS cluster identity resolved once by the
// Cluster Agent through eks:DescribeCluster, so node Agents never derive cluster ownership
// from node-local metadata.
func getEKSClusterIdentity(w http.ResponseWriter, r *http.Request) {
	var spanErr error
	span, _ := tracer.StartSpanFromContext(r.Context(), "cluster_agent.metadata.eks_cluster_identity",
		tracer.ResourceName("eksClusterIdentity"),
	)
	defer func() { span.Finish(tracer.WithError(spanErr)) }()

	identity, err := resolveEKSClusterIdentity(r.Context())
	if err != nil {
		spanErr = err
		api.SetSpanError(w, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	payload, err := json.Marshal(identity)
	if err != nil {
		spanErr = err
		api.SetSpanError(w, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(payload); err != nil {
		log.Debugf("Unable to write EKS cluster identity response: %v", err)
	}
}
