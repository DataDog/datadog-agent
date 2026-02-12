// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/vishvananda/netlink"
)

// QueuedNetworkDeviceError is used to indicate that the new network Device was queued until its namespace handle is
// resolved.
type QueuedNetworkDeviceError struct {
	msg string
}

func (err QueuedNetworkDeviceError) Error() string {
	return err.msg
}

// TcClassifierRequestType represents the type of TC classifier request.
type TcClassifierRequestType int

const (
	// TcNewDeviceRequestType indicates a new device TC classifier request.
	TcNewDeviceRequestType TcClassifierRequestType = iota
	// TcDeviceUpdateRequestType indicates a device update TC classifier request.
	TcDeviceUpdateRequestType
)

// TcClassifierRequest represents an async TC classifier setup request.
type TcClassifierRequest struct {
	RequestType TcClassifierRequestType
	Device      model.NetDevice
}

type tcDeviceKey struct {
	NetNS   uint32
	IfIndex uint32
}

func tcDeviceKeyFromDevice(device model.NetDevice) tcDeviceKey {
	return tcDeviceKey{
		NetNS:   device.NetNS,
		IfIndex: device.IfIndex,
	}
}

// CancelTCClassifierRequestsForDevice marks pending TC classifier requests for the given device as inactive.
// The async worker will skip any queued requests whose device key is not active anymore.
func (tcr *Resolver) CancelTCClassifierRequestsForDevice(device model.NetDevice) {
	key := tcDeviceKeyFromDevice(device)

	tcr.tcRequestsMu.Lock()
	delete(tcr.tcRequestsActive, key)
	tcr.tcRequestsMu.Unlock()
}

func (tcr *Resolver) PushNewTCClassifierRequest(request TcClassifierRequest) {
	key := tcDeviceKeyFromDevice(request.Device)

	tcr.tcRequestsMu.Lock()
	tcr.tcRequestsActive[key] = true
	tcr.tcRequestsMu.Unlock()

	select {
	case <-tcr.ctx.Done():
		// the probe is stopping, do not push the new tc classifier request
		tcr.tcRequestsMu.Lock()
		delete(tcr.tcRequestsActive, key)
		tcr.tcRequestsMu.Unlock()
		return
	case tcr.tcRequests <- request:
		// do nothing
	default:
		tcr.tcRequestsMu.Lock()
		delete(tcr.tcRequestsActive, key)
		tcr.tcRequestsMu.Unlock()
		seclog.Errorf("failed to slot new tc classifier request: %+v", request)
	}
}

func (tcr *Resolver) startSetupNewTCClassifierLoop() {
	for {
		select {
		case <-tcr.ctx.Done():
			return
		case request, ok := <-tcr.tcRequests:
			if !ok {
				return
			}

			key := tcDeviceKeyFromDevice(request.Device)

			tcr.tcRequestsMu.Lock()
			active := tcr.tcRequestsActive[key]
			tcr.tcRequestsMu.Unlock()

			if !active {
				// request was canceled while it was waiting in the queue
				continue
			}

			if err := tcr.setupNewTCClassifier(request.Device); err != nil {
				var qnde QueuedNetworkDeviceError
				var linkNotFound netlink.LinkNotFoundError

				if errors.As(err, &qnde) {
					seclog.Debugf("%v", err)
				} else if errors.As(err, &linkNotFound) {
					seclog.Debugf("link not found while setting up new tc classifier: %v", err)
				} else if errors.Is(err, manager.ErrIdentificationPairInUse) {
					if request.RequestType != TcDeviceUpdateRequestType {
						seclog.Errorf("tc classifier already exists: %v", err)
					} else {
						seclog.Debugf("tc classifier already exists: %v", err)
					}
				} else {
					seclog.Errorf("error setting up new tc classifier on %+v: %v", request.Device, err)
				}

				// clean up the active tracking entry on non-queued errors
				if !errors.As(err, &qnde) {
					tcr.tcRequestsMu.Lock()
					delete(tcr.tcRequestsActive, key)
					tcr.tcRequestsMu.Unlock()
				}
			} else {
				// clean up the active tracking entry on success
				tcr.tcRequestsMu.Lock()
				delete(tcr.tcRequestsActive, key)
				tcr.tcRequestsMu.Unlock()
			}
		}
	}
}

func (tcr *Resolver) setupNewTCClassifier(device model.NetDevice) error {
	// select netns handle
	netns := tcr.ResolveNetworkNamespace(device.NetNS)
	if netns == nil {
		tcr.QueueNetworkDevice(device)
		return QueuedNetworkDeviceError{msg: fmt.Sprintf("device %s is queued until %d is resolved", device.Name, device.NetNS)}
	}

	handle, err := netns.GetNamespaceHandleDup()
	if err != nil || handle == nil {
		tcr.QueueNetworkDevice(device)
		return QueuedNetworkDeviceError{msg: fmt.Sprintf("device %s is queued until %d is resolved", device.Name, device.NetNS)}
	}
	defer func() {
		if cerr := handle.Close(); cerr != nil {
			seclog.Warnf("could not close file [%s]: %s", handle.Name(), cerr)
		}
	}()

	return tcr.tcResolver.SetupNewTCClassifierWithNetNSHandle(device, handle, tcr.manager)
}

func (tcr *Resolver) startTcClassifierLoopGoroutine() {
	// start new tc classifier loop
	tcr.wg.Add(1)
	go func() {
		defer tcr.wg.Done()
		tcr.startSetupNewTCClassifierLoop()
	}()
}
