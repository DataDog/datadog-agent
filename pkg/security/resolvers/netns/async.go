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
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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

// PushNewTCClassifierRequest queues a TC classifier setup request for async processing.
func (tcr *Resolver) PushNewTCClassifierRequest(request TcClassifierRequest) {
	select {
	case <-tcr.ctx.Done():
		// the probe is stopping, do not push the new tc classifier request
		return
	case tcr.tcRequests <- request:
		// do nothing
	default:
		tcr.countError(errorClassQueueFull)
		seclog.Debugf("failed to slot new tc classifier request: %+v", request)
	}
}

// networkNamespaceHandleRequest represents an async request for a handle on the network namespace of a process.
type networkNamespaceHandleRequest struct {
	nsID uint32
	pid  uint32
}

// PushNetworkNamespaceHandleRequest queues a network namespace handle request for async processing.
func (tcr *Resolver) PushNetworkNamespaceHandleRequest(nsID uint32, pid uint32) {
	if !tcr.config.NetworkEnabled || nsID == 0 {
		return
	}

	select {
	case tcr.netnsHandleRequests <- networkNamespaceHandleRequest{nsID: nsID, pid: pid}:
	default:
		// a request is pushed for every event, so a dropped one is pushed again by the next event of its namespace
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

			if err := tcr.setupNewTCClassifier(request.Device); err != nil {
				var qnde QueuedNetworkDeviceError
				if errors.As(err, &qnde) {
					seclog.Debugf("%v", err)
					continue
				}

				tcr.reportTCClassifierError(err, request.Device)
			}
		case request := <-tcr.netnsHandleRequests:
			_, _ = tcr.SaveNetworkNamespaceHandleLazy(request.nsID, func() *utils.NSPath {
				return utils.NewNSPathFromPid(request.pid, utils.NetNsType)
			})
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
