// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cenkalti/backoff/v4"
)

const (
	nodeLabelsCheckInitialInterval = time.Second
	nodeLabelsCheckMaxInterval     = 10 * time.Second
	nodeLabelsCheckMultiplier      = 2.0
	nodeLabelsCheckMaxElapsedTime  = 1 * time.Minute
)

// WaitGetNodeLabels waits for node labels to become available using a backoff retrier
func WaitGetNodeLabels() (map[string]string, error) {
	fetcher := &labelsFetcher{}
	exp := backoff.NewExponentialBackOff()
	exp.InitialInterval = nodeLabelsCheckInitialInterval
	exp.RandomizationFactor = 0
	exp.Multiplier = nodeLabelsCheckMultiplier
	exp.MaxInterval = nodeLabelsCheckMaxInterval
	exp.MaxElapsedTime = nodeLabelsCheckMaxElapsedTime
	exp.Reset()

	err := backoff.RetryNotify(fetcher.fetch, exp, notifyFetchNodeLabels())
	return fetcher.nodeLabels, err
}

type labelsFetcher struct {
	nodeLabels map[string]string
}

func (f *labelsFetcher) fetch() error {
	nodeInfo, err := hostinfo.NewNodeInfo()
	if err != nil {
		return fmt.Errorf("unable to instantiate NodeInfo, err: %w", err)
	}
	f.nodeLabels, err = nodeInfo.GetNodeLabels(context.TODO())
	return err
}

func notifyFetchNodeLabels() backoff.Notify {
	attempt := 0
	return func(err error, delay time.Duration) {
		attempt++
		mins := int(delay.Minutes())
		secs := int(math.Mod(delay.Seconds(), 60))
		log.Warnf("Failed to get node labels (attempt=%d): will retry in %dm%ds: %v", attempt, mins, secs, err)
	}
}
