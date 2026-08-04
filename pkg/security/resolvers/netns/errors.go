// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"errors"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// classes of errors reported by the network namespace resolver
const (
	// errorClassLinkNotFound is reported when an interface is gone before its classifier is attached
	errorClassLinkNotFound = "link_not_found"
	// errorClassNoSuchDevice is reported when an interface is gone while its classifier is attached
	errorClassNoSuchDevice = "no_such_device"
	// errorClassClassifierExists is reported when the eBPF manager already holds a classifier for an interface
	errorClassClassifierExists = "classifier_exists"
	// errorClassUnknown is reported for the failures we can't explain
	errorClassUnknown = "unknown"
)

// classifyTCClassifierError returns the class of a TC classifier setup failure.
func classifyTCClassifierError(err error) string {
	var linkNotFound netlink.LinkNotFoundError

	switch {
	case errors.As(err, &linkNotFound):
		return errorClassLinkNotFound
	case errors.Is(err, unix.ENODEV):
		return errorClassNoSuchDevice
	case errors.Is(err, manager.ErrIdentificationPairInUse):
		return errorClassClassifierExists
	default:
		return errorClassUnknown
	}
}

// reportTCClassifierError reports a TC classifier setup failure. Interfaces are created, renamed and
// deleted faster than classifiers can be attached to them, so the failures we expect are only worth
// a debug line.
func (nr *Resolver) reportTCClassifierError(err error, device model.NetDevice) {
	if class := classifyTCClassifierError(err); class != errorClassUnknown {
		seclog.Debugf("couldn't set up tc classifier on %+v [%s]: %v", device, class, err)
		return
	}

	seclog.Errorf("couldn't set up tc classifier on %+v: %v", device, err)
}
