// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package model holds model related files
package model

import (
	"errors"
	"fmt"
)

var (
	ErrNoImageProvided = errors.New("no image name provided") // ErrNoImageProvided is returned when no image name is provided
)

// WorkloadSelector is a selector used to uniquely indentify the image of a workload
type WorkloadSelector struct {
	Image string
	Tag   string
}

// NewWorkloadSelector returns an initialized instance of a WorkloadSelector
func NewWorkloadSelector(image string, tag string) (WorkloadSelector, error) {
	if image == "" {
		return WorkloadSelector{}, ErrNoImageProvided
	} else if tag == "" {
		tag = "latest"
	}
	return WorkloadSelector{
		Image: image,
		Tag:   tag,
	}, nil
}

// IsReady returns true if the selector is ready
func (ws *WorkloadSelector) IsReady() bool {
	return len(ws.Image) != 0
}

// Match returns true if the input selector matches the current selector
func (ws *WorkloadSelector) Match(selector WorkloadSelector) bool {
	if ws.Tag == "*" || selector.Tag == "*" {
		return ws.Image == selector.Image
	}
	return ws.Image == selector.Image && ws.Tag == selector.Tag
}

// String returns a string representation of a workload selector
func (ws WorkloadSelector) String() string {
	return fmt.Sprintf("[image_name:%s image_tag:%s]", ws.Image, ws.Tag)
}

// ToTags returns a string array representation of a workload selector
func (ws WorkloadSelector) ToTags() []string {
	return []string{
		"image_name:" + ws.Image,
		"image_tag:" + ws.Tag,
	}
}
