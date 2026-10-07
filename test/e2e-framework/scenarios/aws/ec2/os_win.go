// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	componentsos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const windowsTrackSSHActivityScriptPath = "C:/ProgramData/dd-e2e/track-ssh-activity.ps1"

// windowsRegisterTrackSSHActivityTask registers a SYSTEM scheduled task running track-ssh-activity.ps1
// every 5 minutes, which survives reboots. Best effort: a failure prints a warning instead of failing
// the stack. Statements are joined on one line and only use single quotes, to go through the SSH
// command line unchanged.
var windowsRegisterTrackSSHActivityTask = "try { " + strings.Join([]string{
	"$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File " + windowsTrackSSHActivityScriptPath + "'",
	"$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Minutes 5)",
	"$principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest",
	"$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 2) -MultipleInstances IgnoreNew -StartWhenAvailable",
	"Register-ScheduledTask -TaskName 'dd-e2e-track-ssh-activity' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null",
}, "; ") + " } catch { Write-Warning ('Failed to register the SSH activity tracking task: ' + $_) }"

func getWindowsOpenSSHUserData(publicKeyPath string) (string, error) {
	publicKey, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", err
	}

	return buildAWSPowerShellUserData(
			componentsos.WindowsSetupSSHScriptContent,
			windowsPowerShellArgument{name: "authorizedKey", value: string(publicKey)},
		),
		nil
}

type windowsPowerShellArgument struct {
	name  string
	value string
}

func (a windowsPowerShellArgument) String() string {
	return fmt.Sprintf("-%s %s", a.name, a.value)
}

func buildAWSPowerShellUserData(scriptContent string, arguments ...windowsPowerShellArgument) string {
	for _, arg := range arguments {
		scriptContent = strings.ReplaceAll(scriptContent, fmt.Sprintf("$%s", arg.name), fmt.Sprintf("'%s'", arg.value))
	}

	scriptLines := strings.Split(scriptContent, "\n")
	userDataLines := make([]string, 0, len(scriptLines)+6+len(arguments))
	userDataLines = append(userDataLines, "<powershell>")
	for _, line := range scriptLines {
		// indent script lines by one tab
		userDataLines = append(userDataLines, fmt.Sprintf("		%s", line))
	}
	userDataLines = append(userDataLines, "</powershell>")
	userDataLines = append(userDataLines, "<persist>true</persist>")

	return strings.Join(userDataLines, "\n")
}

// installWindowsSSHActivityTracking copies track-ssh-activity.ps1 to the host and schedules it, so that
// the "last-activity" tag is kept up to date while the VM is used over SSH. Unlike on Linux, it is not
// installed through the user data, which is already close to EC2's 16 KB limit on Windows.
func installWindowsSSHActivityTracking(e aws.Environment, host *remote.Host, opts []pulumi.ResourceOption) error {
	dirCmd, err := host.OS.FileManager().CreateDirectoryForFile(windowsTrackSSHActivityScriptPath, false, opts...)
	if err != nil {
		return err
	}

	copyCmd, err := host.OS.FileManager().CopyInlineFile(
		pulumi.String(componentsos.WindowsTrackSSHActivityScriptContent),
		windowsTrackSSHActivityScriptPath,
		utils.MergeOptions(opts, utils.PulumiDependsOn(dirCmd))...)
	if err != nil {
		return err
	}

	_, err = host.OS.Runner().Command(
		e.CommonNamer().ResourceName("track-ssh-activity"),
		&command.Args{Create: pulumi.String(windowsRegisterTrackSSHActivityTask)},
		utils.MergeOptions(opts, utils.PulumiDependsOn(copyCmd))...)
	return err
}
