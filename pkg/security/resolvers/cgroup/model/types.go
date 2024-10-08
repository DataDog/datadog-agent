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

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	ErrNoImageProvided       = errors.New("no image name provided")  // ErrNoImageProvided is returned when no image name is provided
	ErrNoContainerIDProvided = errors.New("no containerID provided") // ErrNoContainerIDProvided is returned when no containerID is provided
)

// WorkloadSelector is a selector used to uniquely indentify the image of a workload
type WorkloadSelector struct {
	Image       string
	Tag         string
	ContainerID string
}

// NewSelector returns an initialized instance of a WorkloadSelector
func NewSelector(image string, tag string, containerID string) (WorkloadSelector, error) {
	if image == "" {
		return WorkloadSelector{}, ErrNoImageProvided
	} else if containerID == "" {
		return WorkloadSelector{}, ErrNoContainerIDProvided
	} else if tag == "" {
		tag = "latest"
	}
	return WorkloadSelector{
		Image:       image,
		Tag:         tag,
		ContainerID: containerID,
	}, nil
}

// NewWorkloadSelector returns an initialized instance of a WorkloadSelector for an image
func NewWorkloadSelector(image string, tag string) (WorkloadSelector, error) {
	return NewSelector(image, tag, "*")
}

// NewContainerSelector returns an initialized instance of a WorkloadSelector for a single container
func NewContainerSelector(containerID string) (WorkloadSelector, error) {
	return NewSelector("*", "*", containerID)
}

// NewWorkloadSelectorFromContainerContext returns a workload selector corresponding to the given container context
func NewWorkloadSelectorFromContainerContext(cc *model.ContainerContext) WorkloadSelector {
	ws := WorkloadSelector{
		Image:       utils.GetTagValue("image_name", cc.Tags),
		Tag:         utils.GetTagValue("image_tag", cc.Tags),
		ContainerID: string(cc.ContainerID),
	}
	if ws.Image == "" {
		ws.Image = "*"
	}
	if ws.Tag == "" {
		ws.Tag = "*"
	}
	return ws
}

// Copy returns a copy of itself
func (ws *WorkloadSelector) Copy() *WorkloadSelector {
	return &WorkloadSelector{
		Image:       ws.Image,
		Tag:         ws.Tag,
		ContainerID: ws.ContainerID,
	}
}

// IsReady returns true if the selector is ready
func (ws *WorkloadSelector) IsReady() bool {
	return len(ws.Image) != 0
}

// Match returns true if the input selector matches the current selector
func (ws *WorkloadSelector) Match(selector WorkloadSelector) bool {
	if ws.ContainerID == "*" || selector.ContainerID == "*" {
		if ws.Tag == "*" || selector.Tag == "*" {
			return ws.Image == selector.Image
		}
		return ws.Image == selector.Image && ws.Tag == selector.Tag
	}
	return ws.Image == selector.Image && ws.Tag == selector.Tag && ws.ContainerID == selector.ContainerID
}

// String returns a string representation of a workload selector
func (ws WorkloadSelector) String() string {
	return fmt.Sprintf("[image_name:%s image_tag:%s container_id:%s]", ws.Image, ws.Tag, ws.ContainerID)
}

// ToTags returns a string array representation of a workload selector, used in profile manger to send stats
func (ws WorkloadSelector) ToTags() []string {
	return []string{
		"image_name:" + ws.Image,
		"image_tag:" + ws.Tag,
	}
}
