// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"errors"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/vishvananda/netlink"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// classes of errors reported by the network namespace resolver
const (
	// errorClassLinkNotFound is reported when an interface is gone before its classifier is attached
	errorClassLinkNotFound = "link_not_found"
	// errorClassNoSuchDevice is reported when an interface is gone while its classifier is attached
	errorClassNoSuchDevice = "no_such_device"
	// errorClassFilterNotFound is reported when the filter of an interface is gone right after being added
	errorClassFilterNotFound = "filter_not_found"
	// errorClassClassifierExists is reported when the eBPF manager already holds a classifier for an interface
	errorClassClassifierExists = "classifier_exists"
	// errorClassQueueFull is reported when a classifier request is dropped because the queue is full
	errorClassQueueFull = "queue_full"
	// errorClassNetlinkSocket is reported when no netlink socket can be opened in a network namespace
	errorClassNetlinkSocket = "netlink_socket"
	// errorClassLinkList is reported when the interfaces of a network namespace can't be listed
	errorClassLinkList = "link_list"
	// errorClassUnknown is reported for the failures we can't explain
	errorClassUnknown = "unknown"
)

// errorClasses lists every class the resolver reports. The counters are built from it upfront so
// that the map is only ever read afterwards, which is what makes it safe to count without a lock.
var errorClasses = []string{
	errorClassLinkNotFound,
	errorClassNoSuchDevice,
	errorClassFilterNotFound,
	errorClassClassifierExists,
	errorClassQueueFull,
	errorClassNetlinkSocket,
	errorClassLinkList,
	errorClassUnknown,
}

func newErrorCounters() map[string]*atomic.Int64 {
	counters := make(map[string]*atomic.Int64, len(errorClasses))
	for _, class := range errorClasses {
		counters[class] = atomic.NewInt64(0)
	}
	return counters
}

// countError counts an error of the provided class. The counters are flushed by SendStats.
func (nr *Resolver) countError(class string) {
	if counter, ok := nr.errorCounters[class]; ok {
		counter.Inc()
	}
}

// sendErrorStats flushes the error counters to the statsd client.
func (nr *Resolver) sendErrorStats() {
	for class, counter := range nr.errorCounters {
		if val := counter.Swap(0); val > 0 {
			_ = nr.client.Count(metrics.MetricNamespaceResolverError, val, []string{"error_type:" + class}, 1.0)
		}
	}
}

// tcFilterNotFoundMsg ends the failure the eBPF manager returns when the filter it just added is
// already missing from the interface it reads back. That one carries no cause to match it on.
const tcFilterNotFoundMsg = "filter not found"

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
	case strings.Contains(err.Error(), tcFilterNotFoundMsg):
		return errorClassFilterNotFound
	default:
		return errorClassUnknown
	}
}

// reportTCClassifierError reports a TC classifier setup failure. Interfaces are created, renamed and
// deleted faster than classifiers can be attached to them, so the failures we expect are only worth
// a debug line.
func (nr *Resolver) reportTCClassifierError(err error, device model.NetDevice) {
	class := classifyTCClassifierError(err)
	nr.countError(class)

	if class != errorClassUnknown {
		seclog.Debugf("couldn't set up tc classifier on %+v [%s]: %v", device, class, err)
		return
	}

	seclog.Errorf("couldn't set up tc classifier on %+v: %v", device, err)
}
