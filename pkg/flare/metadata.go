// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	v5 "github.com/DataDog/datadog-agent/pkg/metadata/v5"
	host "github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func addMetadata(tempDir, hostname, filename string, data []byte) error {
	f := filepath.Join(tempDir, hostname, filename)
	return writeScrubbedFile(f, data)
}

func zipMetadataInventories(tempDir, hostname string) error {
	payload, err := inventories.GetLastPayload()
	if err != nil {
		return err
	}

	return addMetadata(tempDir, hostname, "metadata_inventories.json", payload)
}

func zipMetadataV5(tempDir, hostname string) error {
	ctx := context.Background()
	hostnameData, _ := host.GetWithProvider(ctx)
	payload := v5.GetPayload(ctx, hostnameData)

	data, err := json.MarshalIndent(payload, "", "    ")
	if err != nil {
		return err
	}

	return addMetadata(tempDir, hostname, "metadata_v5.json", data)
}
