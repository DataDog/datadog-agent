// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package dump

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// ActivityDumpRemoteStorageForwarder is a remote storage that forwards dumps to the security-agent
type ActivityDumpRemoteStorageForwarder struct {
	activityDumpHandler ActivityDumpHandler
}

// NewActivityDumpRemoteStorageForwarder returns a new instance of ActivityDumpRemoteStorageForwarder
func NewActivityDumpRemoteStorageForwarder(handler ActivityDumpHandler) (ActivityDumpStorage, error) {
	return &ActivityDumpRemoteStorageForwarder{
		activityDumpHandler: handler,
	}, nil
}

// GetStorageType returns the storage type of the ActivityDumpRemoteStorage
func (storage *ActivityDumpRemoteStorageForwarder) GetStorageType() config.StorageType {
	return config.RemoteStorage
}

// Persist saves the provided buffer to the persistent storage
func (storage *ActivityDumpRemoteStorageForwarder) Persist(request config.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {
	// set activity dump size for current encoding
	ad.Metadata.Size = uint64(raw.Len())

	// generate stream message
	msg := &api.ActivityDumpStreamMessage{
		Dump: ad.ToSecurityActivityDumpMessage(),
		Data: raw.Bytes(),
	}

	// override storage request so that it contains only the current persisted data
	msg.Dump.Storage = []*api.StorageRequestMessage{request.ToStorageRequestMessage(ad.Metadata.Name)}

	if handler := storage.activityDumpHandler; handler != nil {
		handler.HandleActivityDump(msg)
	}

	seclog.Infof("[%s] file for activity dump [%s] was forwarded to the security-agent", request.Format, ad.GetSelectorStr())
	return nil
}

// SendTelemetry sends telemetry for the current storage
func (storage *ActivityDumpRemoteStorageForwarder) SendTelemetry(sender aggregator.Sender) {}
