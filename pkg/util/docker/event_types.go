// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

const (
	// ContainerEventActionStart is the action of starting a docker container
	ContainerEventActionStart = "start"
	// ContainerEventActionDie is the action of stopping a docker container
	ContainerEventActionDie = "die"
	// ContainerEventActionDied is the action of stopping a podman container
	ContainerEventActionDied = "died"
	// ContainerEventActionRename is the action of renaming a docker container
	ContainerEventActionRename = "rename"
	// ContainerEventActionHealthStatus is the action of changing a docker
	// container's health status
	ContainerEventActionHealthStatus = "health_status"

	// ImageEventActionPull is the action of pulling a docker image
	ImageEventActionPull = "pull"
	// ImageEventActionDelete is the action of deleting a docker image
	ImageEventActionDelete = "delete"
	// ImageEventActionTag is the action of tagging a docker image
	ImageEventActionTag = "tag"
	// ImageEventActionUntag is the action of untagging a docker image
	ImageEventActionUntag = "untag"
	// ImageEventActionSbom is the action of getting SBOM information for a docker image
	ImageEventActionSbom = "sbom"
)

var containerEventActions = []string{
	ContainerEventActionStart,
	ContainerEventActionDie,
	ContainerEventActionDied,
	ContainerEventActionRename,
	ContainerEventActionHealthStatus,
}

var imageEventActions = []string{
	ImageEventActionPull,
	ImageEventActionDelete,
	ImageEventActionTag,
	ImageEventActionUntag,
}

// ContainerEvent describes a container event from the docker daemon
type ContainerEvent struct {
	ContainerID   string
	ContainerName string
	ImageName     string
	Action        string
	Timestamp     time.Time
	Attributes    map[string]string
}

// ImageEvent describes an image event from the docker daemon
type ImageEvent struct {
	ImageID   string // In some events this is a sha, in others it's a name with tag
	Action    string
	Timestamp time.Time
	// There are more attributes in the original event. Add them here if they're needed
}

// Errors client might receive
var (
	ErrAlreadySubscribed = errors.New("already subscribed")
	ErrNotSubscribed     = errors.New("not subscribed")
	ErrEventTimeout      = errors.New("timeout on event sending, re-subscribe")
)

// eventSubscriber holds the state for a subscriber
type eventSubscriber struct {
	name                string
	containerEventsChan chan *ContainerEvent
	imageEventsChan     chan *ImageEvent
	cancelChan          chan struct{}
	filter              *containers.Filter
}

// eventStreamState holds the state for event streaming towards subscribers
type eventStreamState struct {
	sync.RWMutex
	subscribers map[string]*eventSubscriber
	cancelChan  chan struct{}
}

func newEventStreamState() *eventStreamState {
	return &eventStreamState{
		subscribers: make(map[string]*eventSubscriber),
		cancelChan:  make(chan struct{}),
	}
}
