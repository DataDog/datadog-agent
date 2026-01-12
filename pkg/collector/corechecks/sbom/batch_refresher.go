// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || (windows && wmi)

package sbom

import (
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type batchRefresher struct {
	ticker *time.Ticker

	wmStore workloadmeta.Component
	proc    *processor
}

var _ containerPeriodicRefresher = (*batchRefresher)(nil)

func newBatchRefresher(period time.Duration, wmStore workloadmeta.Component, proc *processor) *batchRefresher {
	return &batchRefresher{
		ticker: time.NewTicker(period),

		wmStore: wmStore,
		proc:    proc,
	}
}

func (br *batchRefresher) stop() {
	br.ticker.Stop()
}

func (br *batchRefresher) tick() <-chan time.Time {
	return br.ticker.C
}

// step performs a single refresh step
func (br *batchRefresher) step() {
	for _, img := range br.wmStore.ListImages() {
		br.proc.processImageSBOM(img)
	}
}
