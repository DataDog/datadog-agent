// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AutoAddListeners checks if the listener auto is selected and
// adds the docker listener if the host support it
// no effect if the listeners already contain the docker one
// Note: auto listener isn't a real listener but a way to automatically starts other listeners
// TODO: support more listeners (kubelet, ...)
func AutoAddListeners(listeners []config.Listeners) []config.Listeners {
	autoIdx := -1
	for i, l := range listeners {
		switch l.Name {
		case "docker":
			return listeners
		case "auto":
			autoIdx = i
		}
	}
	if autoIdx == -1 {
		return listeners
	}

	// Remove the auto listener element from the listeners slice
	listeners = remove(listeners, autoIdx)

	if IsDockerRunning() == false {
		return listeners
	}

	// Adding listeners
	listeners = addListener(listeners, "docker")
	log.Debugf("returning %d listeners", len(listeners))
	return listeners
}

func addListener(listeners []config.Listeners, name string) []config.Listeners {
	newListener := config.Listeners{Name: name}
	log.Infof("auto adding %q listener", newListener.Name)
	return append(listeners, newListener)
}

// remove the elt at the index
func remove(s []config.Listeners, index int) []config.Listeners {
	s[index] = s[len(s)-1]
	return s[:len(s)-1]
}
