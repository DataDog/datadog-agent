package os

import (
	_ "embed"
)

//go:embed scripts/apt-disable-unattended-upgrades.sh
var APTDisableUnattendedUpgradesScriptContent string

//go:embed scripts/zypper-disable-unattended-upgrades.sh
var ZypperDisableUnattendedUpgradesScriptContent string
