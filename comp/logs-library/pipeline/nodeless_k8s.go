// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package pipeline

import (
	"context"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	nodelessCheckMaxAttempts = 5
	nodelessCheckRetryDelay  = time.Second
)

// isNodelessNode returns true when the current node has the label class=nodeless.
// Retries up to nodelessCheckMaxAttempts times to handle cases where the cluster
// agent or apiserver connection is not yet ready at agent startup.
func isNodelessNode(_ pkgconfigmodel.Reader) bool {
	for attempt := 1; attempt <= nodelessCheckMaxAttempts; attempt++ {
		nodeInfo, err := hostinfo.NewNodeInfo()
		if err != nil {
			log.Debugf("logs-agent: nodeless check attempt %d/%d: could not create NodeInfo: %v", attempt, nodelessCheckMaxAttempts, err)
			time.Sleep(nodelessCheckRetryDelay)
			continue
		}

		labels, err := nodeInfo.GetNodeLabels(context.Background())
		if err != nil {
			log.Debugf("logs-agent: nodeless check attempt %d/%d: could not get node labels: %v", attempt, nodelessCheckMaxAttempts, err)
			time.Sleep(nodelessCheckRetryDelay)
			continue
		}

		isNodeless := labels["class"] == "nodeless"
		log.Infof("logs-agent: nodeless node detection result: isNodeless=%v (class=%q)", isNodeless, labels["class"])
		return isNodeless
	}

	log.Warnf("logs-agent: could not determine nodeless status after %d attempts, defaulting to false", nodelessCheckMaxAttempts)
	return false
}
