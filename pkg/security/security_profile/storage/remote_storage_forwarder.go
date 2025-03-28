// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package storage holds files related to storages for security profiles
package storage

import (
	"bytes"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// ActivityDumpHandler represents an handler for the activity dumps sent by the probe
type ActivityDumpHandler interface {
	HandleActivityDump(dump *api.ActivityDumpStreamMessage)
}

// ActivityDumpRemoteStorageForwarder is a remote storage that forwards dumps to the security-agent
type ActivityDumpRemoteStorageForwarder struct {
	activityDumpHandler ActivityDumpHandler
}

// NewActivityDumpRemoteStorageForwarder returns a new instance of ActivityDumpRemoteStorageForwarder
func NewActivityDumpRemoteStorageForwarder(handler ActivityDumpHandler) (*ActivityDumpRemoteStorageForwarder, error) {
	return &ActivityDumpRemoteStorageForwarder{
		activityDumpHandler: handler,
	}, nil
}

// GetStorageType returns the storage type of the ActivityDumpRemoteStorage
func (storage *ActivityDumpRemoteStorageForwarder) GetStorageType() config.StorageType {
	return config.RemoteStorage
}

// Persist saves the provided buffer to the persistent storage
func (storage *ActivityDumpRemoteStorageForwarder) Persist(request config.StorageRequest, p *profile.Profile, raw *bytes.Buffer) error {
	// set activity dump size for current encoding
	p.Metadata.Size = uint64(raw.Len())

	// generate stream message
	msg := &api.ActivityDumpStreamMessage{
		Dump: p.ToSecurityActivityDumpMessage(p.Metadata.End.Sub(p.Metadata.Start), map[config.StorageFormat][]config.StorageRequest{
			request.Format: {request},
		}),
		Data: raw.Bytes(),
	}

	if handler := storage.activityDumpHandler; handler != nil {
		handler.HandleActivityDump(msg)
	}

	seclog.Infof("[%s] file for activity dump [%s] was forwarded to the security-agent", request.Format, p.GetSelectorStr())
	return nil
}

// SendTelemetry sends telemetry for the current storage
func (storage *ActivityDumpRemoteStorageForwarder) SendTelemetry(_ statsd.ClientInterface) {}
