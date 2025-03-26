// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package runtime holds runtime related files
package runtime

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

//nolint:unused // TODO(SEC) Fix unused linter
func printStorageRequestMessage(prefix string, storage *api.StorageRequestMessage) {
	fmt.Printf("%so file: %s\n", prefix, storage.GetFile())
	fmt.Printf("%s  format: %s\n", prefix, storage.GetFormat())
	fmt.Printf("%s  storage type: %s\n", prefix, storage.GetType())
	fmt.Printf("%s  compression: %v\n", prefix, storage.GetCompression())
}
