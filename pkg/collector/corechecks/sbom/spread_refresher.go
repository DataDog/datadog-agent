// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || (windows && wmi)

package sbom

import (
	"slices"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type spreadRefresher struct {
	ticker       *time.Ticker
	refreshTimes map[workloadmeta.EntityID]time.Time

	wmStore workloadmeta.Component
	proc    *processor
}

var _ containerPeriodicRefresher = (*spreadRefresher)(nil)

const spreadSteps = 10

func newSpreadRefresher(period time.Duration, wmStore workloadmeta.Component, proc *processor) *spreadRefresher {
	innerPeriod := period / spreadSteps

	return &spreadRefresher{
		ticker: time.NewTicker(innerPeriod),

		wmStore: wmStore,
		proc:    proc,
	}
}

func (br *spreadRefresher) stop() {
	br.ticker.Stop()
}

func (br *spreadRefresher) tick() <-chan time.Time {
	return br.ticker.C
}

// step performs a single refresh step
func (br *spreadRefresher) step() {
	images := br.wmStore.ListImages()

	// first step: we compute the refresh times for all images
	// and sort them by refresh time

	workingSet := make([]*imageWithRefreshTime, 0, len(images))
	for _, img := range images {
		id := img.EntityID
		refreshTime, ok := br.refreshTimes[id]
		if !ok {
			refreshTime = time.Time{}
		}
		workingSet = append(workingSet, &imageWithRefreshTime{
			image:       img,
			refreshTime: refreshTime,
		})
	}
	slices.SortFunc(workingSet, func(a, b *imageWithRefreshTime) int {
		return a.refreshTime.Compare(b.refreshTime)
	})

	// second step: we process the oldest images
	amountOfImagesToProcess := len(images) / spreadSteps
	for _, img := range workingSet[:amountOfImagesToProcess] {
		br.proc.processImageSBOM(img.image)
		img.refreshTime = time.Now()
	}

	// third step: we update the refresh times map for next step
	newRefreshTimes := make(map[workloadmeta.EntityID]time.Time, len(images))
	for _, img := range workingSet {
		newRefreshTimes[img.image.EntityID] = img.refreshTime
	}

	br.refreshTimes = newRefreshTimes
}

type imageWithRefreshTime struct {
	image       *workloadmeta.ContainerImageMetadata
	refreshTime time.Time
}
