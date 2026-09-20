// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

const (
	// crdWaitInitialInterval is the delay before the first TagRule CRD retry.
	crdWaitInitialInterval = 1 * time.Second
	// crdWaitMaxInterval caps the CRD retry backoff.
	crdWaitMaxInterval = 5 * time.Minute
)

var startOnce sync.Once

// Start starts the tag-rule controller in the Cluster Agent. It is gated on
// the config (cluster_agent.tag_rules.enabled) by the caller, on leadership
// by the controller itself, and on the TagRule CRD by a retry loop: the
// controller starts only once the CRD is present, without crashing the
// Cluster Agent while it is missing.
func Start(ctx context.Context, isLeader func() bool, logger log.Component, datadogConfig config.Component) error {
	apiCl, err := apiserver.GetAPIClient()
	if err != nil {
		return fmt.Errorf("tag-rule controller: getting Kubernetes API client: %w", err)
	}

	startOnce.Do(func() {
		go func() {
			if !waitForTagRuleCRD(ctx, apiCl.DynamicCl, logger) {
				return // context cancelled while waiting
			}
			controller, err := NewController(apiCl.DynamicCl,
				WithLogger(slogLogger(logger)),
				WithIntervals(
					datadogConfig.GetDuration("cluster_agent.tag_rules.status_interval"),
					datadogConfig.GetDuration("cluster_agent.tag_rules.resync_interval"),
				),
				WithLeaderCheck(isLeader),
			)
			if err != nil {
				logger.Errorf("Cannot create tag-rule controller: %v", err)
				return
			}
			logger.Info("Starting tag-rule controller")
			if err := controller.Run(ctx); err != nil {
				logger.Errorf("Tag-rule controller stopped: %v", err)
			}
		}()
	})

	return nil
}

// waitForTagRuleCRD blocks until the TagRule resource can be listed (the CRD
// is installed and served), retrying with exponential backoff. Returns false
// if the context is cancelled first.
func waitForTagRuleCRD(ctx context.Context, dynClient dynamic.Interface, logger log.Component) bool {
	delay := crdWaitInitialInterval
	for attempt := 1; ; attempt++ {
		if _, err := dynClient.Resource(tagRuleGVR).List(ctx, metav1.ListOptions{}); err == nil {
			if attempt > 1 {
				logger.Infof("TagRule CRD is now available (after %d attempts)", attempt)
			}
			return true
		} else if ctx.Err() != nil {
			return false
		} else {
			logger.Warnf("TagRule CRD not available yet (attempt %d, retrying in %s): %v", attempt, delay, err)
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
		if delay *= 2; delay > crdWaitMaxInterval {
			delay = crdWaitMaxInterval
		}
	}
}

// slogLogger adapts the Cluster Agent log component into the controller's
// slog logger, so controller logs flow through the process log pipeline.
func slogLogger(logger log.Component) *slog.Logger {
	return slog.New(slog.NewTextHandler(logWriter{logger}, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

type logWriter struct {
	logger log.Component
}

func (w logWriter) Write(p []byte) (int, error) {
	w.logger.Info(string(bytes.TrimSpace(p)))
	return len(p), nil
}
