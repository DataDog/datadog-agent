// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
	identitytypes "github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/eksidentity"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

// resolveEKSClusterIdentity is injectable for tests.
var resolveEKSClusterIdentity = eksidentity.Resolve

// identityAnswer is the identity endpoint's answer. Its field reads into
// identity.ClusterIdentity on the node.
type identityAnswer struct {
	ClusterARN string `json:"cluster_arn"`
}

// getEKSClusterIdentity returns the authoritative EKS cluster identity resolved once by the
// Cluster Agent through eks:DescribeCluster, so node Agents never derive cluster ownership
// from node-local metadata.
func getEKSClusterIdentity(w http.ResponseWriter, r *http.Request) {
	var spanErr error
	span, _ := tracer.StartSpanFromContext(r.Context(), "cluster_agent.metadata.eks_cluster_identity",
		tracer.ResourceName("eksClusterIdentity"),
	)
	defer func() { span.Finish(tracer.WithError(spanErr)) }()
	// A panic reaches the server's RecoveryHandler, which answers 500 on this writer: mark it
	// final, so nodes do not take it for a proxy error and retry it.
	defer func() {
		if r := recover(); r != nil {
			w.Header().Set(identitytypes.RetryLaterHeader, "false")
			panic(r)
		}
	}()

	identity, err := resolveEKSClusterIdentity(r.Context())
	if err != nil {
		// Every answer the Cluster Agent produces carries identitytypes.RetryLaterHeader, true or
		// false, so a node can tell it from an unmarked 5xx from a proxy or a mesh sidecar,
		// which it retries.
		//
		// Not running on EKS, or no cluster name to look up, is a permanent and expected
		// outcome on other distributions. Report it as "not found" without marking the
		// span errored so non-EKS clusters do not emit a steady stream of false errors.
		if errors.Is(err, eksidentity.ErrNotEKS) || errors.Is(err, eksidentity.ErrClusterNameUnavailable) || errors.Is(err, eksidentity.ErrClusterUnverified) {
			w.Header().Set(identitytypes.RetryLaterHeader, "false")
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		// Not determined yet (the Cluster Agent is still starting): the node may try again
		// within its startup budget.
		if errors.Is(err, eksidentity.ErrNotDetermined) {
			w.Header().Set(identitytypes.RetryLaterHeader, "true")
			log.Debugf("EKS cluster identity is not available yet: %v", err)
			http.Error(w, "EKS cluster identity not available yet", http.StatusServiceUnavailable)
			return
		}
		// A failure the Cluster Agent caches is final for nodes, unless its cache entry expires
		// within retryLaterWindow: then the next DescribeCluster happens while a starting node
		// still waits, and the answer can change. A failure cached longer would only send the
		// node the same answer again for its whole budget.
		retry := "false"
		if left, cached := cachedFailureRetryIn(err); cached && left <= retryLaterWindow {
			retry = "true"
		}
		w.Header().Set(identitytypes.RetryLaterHeader, retry)
		spanErr = err
		api.SetSpanError(w, err)
		// The underlying error can be a raw AWS SDK message carrying the Cluster Agent's
		// assumed-role ARN, account ID, and request ID. Record the detail on the span, but return
		// a generic body to the node Agent. The failure is logged at Warn once, where it is
		// resolved and cached (pkg/util/eks), not once per node request.
		log.Debugf("Unable to resolve EKS cluster identity: %v", err)
		http.Error(w, "EKS cluster identity unavailable", http.StatusServiceUnavailable)
		return
	}

	// Only the ARN goes on the wire: the node validates it and derives the account and Region
	// from it (identity.ClusterIdentity.Tags). A struct with one string field always marshals.
	payload, _ := json.Marshal(identityAnswer{ClusterARN: identity.ClusterARN})
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(payload); err != nil {
		log.Debugf("Unable to write EKS cluster identity response: %v", err)
	}
}

// cachedFailureRetryIn is eks.RetryIn. Variable for tests.
var cachedFailureRetryIn = eks.RetryIn

// retryLaterWindow is how soon a cached failure must expire for nodes to ask again. A node waits
// at most 10s at startup and asks once a second, so a failure that the Cluster Agent retries
// within 5s is retried at least once by a node that started with it, without making nodes ask
// for most of the 30s a transient failure stays cached.
const retryLaterWindow = 5 * time.Second
