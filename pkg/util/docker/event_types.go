// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

import "time"

// ContainerEvent describes an event from the docker daemon
type ContainerEvent struct {
	ContainerID   string
	ContainerName string
	ImageName     string
	Action        string
	Timestamp     time.Time
	Attributes    map[string]string
}

// ContainerEntityName returns the event's container as a tagger entity name
func (ev *ContainerEvent) ContainerEntityName() string {
	return ContainerIDToEntityName(ev.ContainerID)
}
