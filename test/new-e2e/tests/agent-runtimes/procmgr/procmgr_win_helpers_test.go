// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"encoding/base64"
)

// writeProcessesDYamlContent writes a processes.d YAML file as UTF-8 without a BOM.
// Set-Content -Encoding utf8 adds a BOM on Windows PowerShell 5.1, which breaks dd-procmgr parsing.
func writeProcessesDYamlContent(yamlPath, content string) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	return psRemote(
		`$ErrorActionPreference='Stop'; $p='%s'; $b = [Convert]::FromBase64String('%s'); [IO.File]::WriteAllBytes($p, $b)`,
		yamlPath, b64,
	)
}
