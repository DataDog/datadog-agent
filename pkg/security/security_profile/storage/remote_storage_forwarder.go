// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package storage holds files related to storages for security profiles
package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
)

// ActivityDumpRemoteStorageForwarder is a remote storage that forwards dumps to the security-agent
type ActivityDumpRemoteStorageForwarder struct {
	activityDumpHandler backend.ActivityDumpHandler
}

// NewActivityDumpRemoteStorageForwarder returns a new instance of ActivityDumpRemoteStorageForwarder
func NewActivityDumpRemoteStorageForwarder(handler backend.ActivityDumpHandler) (*ActivityDumpRemoteStorageForwarder, error) {
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
	selector := p.GetWorkloadSelector()

	// set activity dump size for current encoding
	p.Metadata.Size = uint64(raw.Len())

	p.Header.DDTags = strings.Join(p.GetTags(), ",")
	// marshal event metadata
	headerData, err := json.Marshal(p.Header)
	if err != nil {
		return errors.New("couldn't marshall event metadata")
	}

	if storage.activityDumpHandler == nil {
		return nil
	}

	err = storage.activityDumpHandler.HandleActivityDump(selector.Image, selector.Tag, headerData, raw.Bytes())
	seclog.Infof("[%s] file for activity dump [%s] was forwarded to the activity dump handler", request.Format, selector)
	return err
}

// SendTelemetry sends telemetry for the current storage
func (storage *ActivityDumpRemoteStorageForwarder) SendTelemetry(_ statsd.ClientInterface) {}
