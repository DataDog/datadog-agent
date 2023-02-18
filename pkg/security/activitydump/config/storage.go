// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

func init() {
	for _, format := range AllStorageFormats() {
		strToFormats[format.String()] = format
	}
	for _, storage := range AllStorageTypes() {
		strToTypes[storage.String()] = storage
	}
}

// StorageRequest is used to request a type of storage for a dump
type StorageRequest struct {
	Type        StorageType
	Format      StorageFormat
	Compression bool

	// LocalStorage specific parameters
	OutputDirectory string
}

// NewStorageRequest returns a new StorageRequest instance
func NewStorageRequest(storageType StorageType, format StorageFormat, compression bool, outputDirectory string) StorageRequest {
	return StorageRequest{
		Type:            storageType,
		Format:          format,
		Compression:     compression,
		OutputDirectory: outputDirectory,
	}
}

// ParseStorageRequests parses storage requests from a gRPC call
func ParseStorageRequests(requests *api.StorageRequestParams) ([]StorageRequest, error) {
	parsedRequests := make([]StorageRequest, 0, len(requests.GetRemoteStorageFormats())+len(requests.GetLocalStorageFormats()))
	formats, err := ParseStorageFormats(requests.GetLocalStorageFormats())
	if err != nil {
		return nil, err
	}
	for _, format := range formats {
		parsedRequests = append(parsedRequests, NewStorageRequest(
			LocalStorage,
			format,
			requests.GetLocalStorageCompression(),
			requests.GetLocalStorageDirectory(),
		))
	}

	// add remote storage requests
	formats, err = ParseStorageFormats(requests.GetRemoteStorageFormats())
	if err != nil {
		return nil, err
	}
	for _, format := range formats {
		parsedRequests = append(parsedRequests, NewStorageRequest(
			RemoteStorage,
			format,
			requests.GetRemoteStorageCompression(),
			"",
		))
	}

	return parsedRequests, nil
}

// ToStorageRequestMessage returns an api.StorageRequestMessage from the StorageRequest
func (sr *StorageRequest) ToStorageRequestMessage(filename string) *api.StorageRequestMessage {
	return &api.StorageRequestMessage{
		Compression: sr.Compression,
		Type:        sr.Type.String(),
		Format:      sr.Format.String(),
		File:        sr.GetOutputPath(filename),
	}
}

// GetOutputPath returns the output path to the file in the storage
func (sr *StorageRequest) GetOutputPath(filename string) string {
	var compressionSuffix string
	if sr.Compression {
		compressionSuffix = ".gz"
	}
	return path.Join(sr.OutputDirectory, filename) + "." + sr.Format.String() + compressionSuffix
}

// StorageFormat is used to define the format of a dump
type StorageFormat string

func (sf StorageFormat) String() string {
	return string(sf)
}

var (
	// JSON is used to request the JSON format
	JSON StorageFormat = "json"
	// PROTOBUF is used to request the protobuf format
	PROTOBUF StorageFormat = "protobuf"
	// DOT is used to request the dot format
	DOT StorageFormat = "dot"
	// Profile is used to request the Secl profile format
	Profile StorageFormat = "profile"

	strToFormats = make(map[string]StorageFormat)
)

// AllStorageFormats returns the list of supported formats
func AllStorageFormats() []StorageFormat {
	return []StorageFormat{JSON, PROTOBUF, DOT, Profile}
}

// ParseStorageFormat returns a storage format from a string input
func ParseStorageFormat(input string) (StorageFormat, error) {
	if len(input) > 0 && input[0] == '.' {
		input = input[1:]
	}
	format, ok := strToFormats[input]
	if !ok {
		return "", fmt.Errorf("%s: unknown storage format, available options are %v", input, AllStorageFormats())
	}
	return format, nil
}

// ParseStorageFormats returns a list of storage formats from a list of strings
func ParseStorageFormats(input []string) ([]StorageFormat, error) {
	output := make([]StorageFormat, 0, len(input))
	for _, in := range input {
		format, err := ParseStorageFormat(in)
		if err != nil {
			return nil, err
		}
		output = append(output, format)
	}
	return output, nil
}

// StorageType is used to define the type of storage
type StorageType int

const (
	// LocalStorage is used to request a local storage
	LocalStorage StorageType = iota
	// RemoteStorage is used to request a remote storage
	RemoteStorage
)

// AllStorageTypes returns the list of supported storage types
func AllStorageTypes() []StorageType {
	return []StorageType{LocalStorage, RemoteStorage}
}

var (
	strToTypes = make(map[string]StorageType)
)

func (st StorageType) String() string {
	switch st {
	case LocalStorage:
		return "local_storage"
	case RemoteStorage:
		return "remote_storage"
	default:
		return ""
	}
}

// ParseStorageType returns a storage type from its string representation
func ParseStorageType(input string) (StorageType, error) {
	storageType, ok := strToTypes[input]
	if !ok {
		return -1, fmt.Errorf("unknown storage type [%s]", input)
	}
	return storageType, nil
}
