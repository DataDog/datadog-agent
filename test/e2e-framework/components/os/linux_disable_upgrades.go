// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

import (
	_ "embed"
)

//go:embed scripts/apt-disable-unattended-upgrades.sh
var APTDisableUnattendedUpgradesScriptContent string

//go:embed scripts/ssh-allow-sftp-root.sh
var SSHAllowSFTPRootScriptContent string

//go:embed scripts/zypper-disable-unattended-upgrades.sh
var ZypperDisableUnattendedUpgradesScriptContent string
