// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

func zipRemoteConfigDB(tempDir, hostname string) error {
	srcPath := filepath.Join(config.Datadog.GetString("run_path"), "remote-config.db")
	dstPath := filepath.Join(tempDir, hostname, "remote-config.db")
	return util.CopyFileAll(srcPath, dstPath)
}
