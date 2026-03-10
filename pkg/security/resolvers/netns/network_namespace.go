// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package netns holds netns related files
package netns

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"
)

var (
	// ErrNoNetworkNamespaceHandle is used to indicate that we haven't resolved a handle for the requested network
	// namespace yet.
	ErrNoNetworkNamespaceHandle = errors.New("no network namespace handle")
)

// NetworkNamespace is used to hold a handle to a network namespace
type NetworkNamespace struct {
	sync.RWMutex

	// nsID is the network namespace ID of the current network namespace.
	nsID uint32

	// handle is the network namespace handle that points to the current network namespace. This handle is used by the
	// manager to create a netlink socket inside the network namespace in which lives the network interfaces we want to
	// monitor.
	handle *os.File

	// networkDevicesQueue is the list of devices that we have detected at runtime, but to which we haven't been able
	// to attach a probe yet. These devices will be dequeued once we capture a network namespace handle, or when the
	// current network namespace expires (see the timeout below).
	networkDevicesQueue []model.NetDevice

	// lonelyTimeout indicates that we have been able to capture a handle for this namespace, but we are yet to see an
	// interface in this namespace. The handle of this namespace will be released if we don't see an interface by the
	// time this timeout expires.
	lonelyTimeout time.Time
}

// ID returns the network namespace ID
func (nn *NetworkNamespace) ID() uint32 {
	return nn.nsID
}

// NewNetworkNamespace returns a new NetworkNamespace instance
func NewNetworkNamespace(nsID uint32) *NetworkNamespace {
	return &NetworkNamespace{
		nsID: nsID,
	}
}

// NewNetworkNamespaceWithPath returns a new NetworkNamespace instance from a path.
func NewNetworkNamespaceWithPath(nsID uint32, nsPath *utils.NSPath) (*NetworkNamespace, error) {
	netns := NewNetworkNamespace(nsID)
	if err := netns.openHandle(nsPath); err != nil {
		return nil, err
	}
	return netns, nil
}

// openHandle tries to create a network namespace handle with the provided thread ID
func (nn *NetworkNamespace) openHandle(nsPath *utils.NSPath) error {
	nn.Lock()
	defer nn.Unlock()

	// check that the handle matches the expected netns ID
	threadNetnsID, err := nsPath.GetNSID()
	if err != nil {
		return err
	}
	if threadNetnsID != nn.nsID {
		// The reason why this can happen is that a process can hold a socket in a different network namespace. This is
		// the case for the Docker Embedded DNS server: a socket is created in the container namespace, but the thead
		// holding the socket jumps back to the host network namespace. Unfortunately this code is racy: ideally we'd
		// like to lock the network namespace of the thread in place until we fetch both the netns ID and the handle,
		// but afaik that's not possible (without freezing the process or its cgroup ...).
		return fmt.Errorf("the provided doesn't match the expected netns ID: got %d, expected %d", threadNetnsID, nn.nsID)
	}

	handle, err := os.Open(nsPath.GetPath())
	if err != nil {
		return err
	}
	nn.handle = handle
	return nil
}

// GetNamespaceHandleDup duplicates the network namespace handle and returns it. WARNING: it is up to the caller of this
// function to close the duplicated network namespace handle. Failing to close a network namespace handle may lead to
// leaking the network namespace.
func (nn *NetworkNamespace) GetNamespaceHandleDup() (*os.File, error) {
	nn.Lock()
	defer nn.Unlock()

	return nn.getNamespaceHandleDup()
}

// getNamespaceHandleDup is an internal function (see GetNamespaceHandleDup)
func (nn *NetworkNamespace) getNamespaceHandleDup() (*os.File, error) {
	if nn.handle == nil {
		return nil, ErrNoNetworkNamespaceHandle
	}

	// duplicate the file descriptor to avoid race conditions with the resync
	dup, err := unix.FcntlInt(nn.handle.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(dup), nn.handle.Name()), nil
}

// dequeueNetworkDevices dequeues the devices in the current network devices queue.
func (nn *NetworkNamespace) dequeueNetworkDevices(tcResolver *tc.Resolver, manager *manager.Manager) {
	nn.Lock()
	defer nn.Unlock()

	if len(nn.networkDevicesQueue) == 0 {
		return
	}

	// make a copy of the network namespace handle to make sure we don't poison our internal cache if the eBPF library
	// modifies the handle.
	handle, err := nn.getNamespaceHandleDup()
	if err != nil {
		return
	}

	defer func() {
		if cerr := handle.Close(); cerr != nil {
			seclog.Warnf("could not close file [%s]: %s", handle.Name(), cerr)
		}
	}()

	for _, queuedDevice := range nn.networkDevicesQueue {
		if err = tcResolver.SetupNewTCClassifierWithNetNSHandle(queuedDevice, handle, manager); err != nil {
			seclog.Errorf("error setting up new tc classifier on queued Device: %v", err)
		}
	}
	nn.flushNetworkDevicesQueue()
}

func (nn *NetworkNamespace) queueNetworkDevice(device model.NetDevice) {
	nn.Lock()
	defer nn.Unlock()

	nn.networkDevicesQueue = append(nn.networkDevicesQueue, device)
}

func (nn *NetworkNamespace) flushNetworkDevicesQueue() {
	// flush the network devices queue
	nn.networkDevicesQueue = nil
}

func (nn *NetworkNamespace) close() error {
	return nn.handle.Close()
}

func (nn *NetworkNamespace) hasValidHandle() bool {
	return nn.handle != nil
}
