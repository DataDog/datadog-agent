// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"path"
	strings "strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
)

// ActivityDumpRemoteStorageForwarder is a remote storage that forwards dumps to the security-agent
type ActivityDumpRemoteStorageForwarder struct {
	probe *Probe
}

// NewActivityDumpRemoteStorageForwarder returns a new instance of ActivityDumpRemoteStorageForwarder
func NewActivityDumpRemoteStorageForwarder(p *Probe) (ActivityDumpStorage, error) {
	return &ActivityDumpRemoteStorageForwarder{
		probe: p,
	}, nil
}

// GetStorageType returns the storage type of the ActivityDumpRemoteStorage
func (storage *ActivityDumpRemoteStorageForwarder) GetStorageType() dump.StorageType {
	return dump.RemoteStorage
}

// Persist saves the provided buffer to the persistent storage
func (storage *ActivityDumpRemoteStorageForwarder) Persist(request dump.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {

	if request.Compression {
		var tmpBuf bytes.Buffer
		zw := gzip.NewWriter(&tmpBuf)
		zw.Name = strings.TrimSuffix(path.Base(request.GetOutputPath(ad.DumpMetadata.Name)), ".gz")
		zw.ModTime = time.Now()
		if _, err := zw.Write(raw.Bytes()); err != nil {
			return fmt.Errorf("couldn't compress activity dump: %w", err)
		}
		if err := zw.Flush(); err != nil {
			return fmt.Errorf("couldn't compress activity dump: %w", err)
		}
		if err := zw.Close(); err != nil {
			return fmt.Errorf("couldn't compress activity dump: %w", err)
		}
		raw = &tmpBuf
	}

	// set activity dump size for current encoding
	ad.DumpMetadata.Size = uint64(len(raw.Bytes()))

	// generate stream message
	msg := &api.ActivityDumpStreamMessage{
		Dump:         ad.ToSecurityActivityDumpMessage(),
		IsCompressed: request.Compression,
		Data:         raw.Bytes(),
	}

	// override storage request so that it contains only the current persisted data
	msg.Dump.Storage = []*api.StorageRequestMessage{request.ToStorageRequestMessage(ad.DumpMetadata.Name)}

	storage.probe.DispatchActivityDump(msg)

	seclog.Infof("[%s] file for activity dump [%s] was forwarded to the security-agent", request.Format, ad.GetSelectorStr())
	return nil
}

// SendTelemetry sends telemetry for the current storage
func (storage *ActivityDumpRemoteStorageForwarder) SendTelemetry(sender aggregator.Sender) {}
