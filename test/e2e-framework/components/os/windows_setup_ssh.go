package os

import (
	_ "embed"
)

//go:embed scripts/setup-ssh.ps1
var WindowsSetupSSHScriptContent string
