// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy

// Package sbom contains the sbom check
package sbom

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	imageMetric = "datadog.agent.sbom_images"
)

// imageTelemetry contains metrics that must always be sent
type imageTelemetry struct {
	sender                sender.Sender
	imageWithoutSBOMCount float64
	imageWithSBOMCount    float64
	imageHasSBOM          map[string]bool
}

func newImageTelemetry(sender sender.Sender) *imageTelemetry {
	return &imageTelemetry{sender: sender, imageHasSBOM: make(map[string]bool)}
}

func (t *imageTelemetry) observeNewImage(img *workloadmeta.ContainerImageMetadata) {
	knownHasSBOM, ok := t.imageHasSBOM[img.ID]
	newHasSBOM := img.SBOM != nil
	if ok {
		// If the image is already known
		if newHasSBOM == knownHasSBOM {
			// If there is no change, don't do anything
			return
		}
		// If there is a change then update both counters accordingly
		t.imageWithSBOMCount += and(newHasSBOM, !knownHasSBOM)    // +1 for transition NoSBOM=>SBOM else -1
		t.imageWithoutSBOMCount += and(!newHasSBOM, knownHasSBOM) // +1 for transition => SBOM=>NoSBOM else -1
		t.sendMetric(newHasSBOM)
	} else {
		// If the image is not known, add it to the map and update the right counter (with or without sbom)
		t.imageHasSBOM[img.ID] = newHasSBOM
		updateCountForNewImage(t, newHasSBOM)
		t.sendMetric(newHasSBOM)
	}
}

// and returns 1 if A and B otherwise -1
func and(a, b bool) float64 {
	if a && b {
		return 1
	}
	return -1
}

// updateCountForNewImage updates the counts for unknown images
func updateCountForNewImage(t *imageTelemetry, newHasSBOM bool) {
	if newHasSBOM {
		t.imageWithSBOMCount++
		return
	}
	t.imageWithoutSBOMCount++
}

// sendMetric submits metrics
func (t *imageTelemetry) sendMetric(sendImageWithSBOM bool) {
	sbomTag := fmt.Sprintf("with_sbom:%v", sendImageWithSBOM)
	value := t.imageWithSBOMCount
	if !sendImageWithSBOM {
		value = t.imageWithoutSBOMCount
	}
	t.sender.Gauge(imageMetric, value, "", []string{sbomTag})
	t.sender.Commit()
}

func (t *imageTelemetry) unobserveImage(img *workloadmeta.ContainerImageMetadata) {
	hasSBOM, ok := t.imageHasSBOM[img.ID]
	if !ok {
		return
	}
	if hasSBOM {
		t.imageWithSBOMCount--
	} else {
		t.imageWithoutSBOMCount--
	}
	t.sendMetric(hasSBOM)
	delete(t.imageHasSBOM, img.ID)
}
