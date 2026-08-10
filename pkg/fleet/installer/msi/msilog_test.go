// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"embed"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/unicode"
)

//go:embed testdata
var logFilesFS embed.FS

func TestFindAllIndexWithContext(t *testing.T) {
	data, err := logFilesFS.ReadFile("testdata/wixfailwhendeferred.log")
	require.NoError(t, err)

	r := regexp.MustCompile("returned actual error")
	decodedLogsBytes, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Bytes(data)
	require.NoError(t, err)

	matches := FindAllIndexWithContext(r, decodedLogsBytes, 2, 1)

	expectedMatches := []string{
		`Action start 2:10:53: WixRemoveFoldersEx.
WixRemoveFoldersEx:  Error 0x80070057: Missing folder property: dd_PROJECTLOCATION_0 for row: RemoveFolderEx
CustomAction WixRemoveFoldersEx returned actual error code 1603 but will be translated to success due to continue marking
Action ended 2:10:53: WixRemoveFoldersEx. Return value 1.`,
		`Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.PrepareDecompressPythonDistributions
CA: 02:10:56: PrepareDecompressPythonDistributions. Could not set the progress bar size
CustomAction PrepareDecompressPythonDistributions returned actual error code 1603 but will be translated to success due to continue marking
Action ended 2:10:56: PrepareDecompressPythonDistributions. Return value 1.`,
		`SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.StartDDServices
CustomAction WixFailWhenDeferred returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)
Action ended 2:11:49: InstallFinalize. Return value 3.`,
	}
	for i, expectedMatch := range expectedMatches {
		text := strings.ReplaceAll(string(decodedLogsBytes[matches[i].start:matches[i].end]), "\r", "")
		require.Equal(t, expectedMatch, text)
	}
}

func TestFindAllIndexWithContextOutOfRangeDoesntFail(t *testing.T) {
	data, err := logFilesFS.ReadFile("testdata/wixfailwhendeferred.log")
	require.NoError(t, err)

	r := regexp.MustCompile("returned actual error")
	decodedLogsBytes, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Bytes(data)
	require.NoError(t, err)

	matches := FindAllIndexWithContext(r, decodedLogsBytes, -21, -1435)
	expectedMatches := []string{
		`CustomAction WixRemoveFoldersEx returned actual error code 1603 but will be translated to success due to continue marking`,
		`CustomAction PrepareDecompressPythonDistributions returned actual error code 1603 but will be translated to success due to continue marking`,
		`CustomAction WixFailWhenDeferred returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)`,
	}
	for i, expectedMatch := range expectedMatches {
		text := strings.ReplaceAll(string(decodedLogsBytes[matches[i].start:matches[i].end]), "\r", "")
		require.Equal(t, expectedMatch, text)
	}
}

func TestCombineRanges(t *testing.T) {
	tests := map[string]struct {
		input    []logFileProcessor
		expected []TextRange
	}{
		"overlap left": {
			input: []logFileProcessor{
				func(_ []byte) []TextRange {
					return []TextRange{{1, 5}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{4, 7}}
				}},
			expected: []TextRange{{1, 7}},
		},
		"overlap right": {
			input: []logFileProcessor{
				func(_ []byte) []TextRange {
					return []TextRange{{2, 4}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{1, 3}}
				}},
			expected: []TextRange{{1, 4}},
		},
		"no overlap": {
			input: []logFileProcessor{
				func(_ []byte) []TextRange {
					return []TextRange{{2, 4}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{5, 10}}
				}},
			expected: []TextRange{{2, 4}, {5, 10}},
		},
		"full overlap": {
			input: []logFileProcessor{
				func(_ []byte) []TextRange {
					return []TextRange{{2, 4}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{1, 10}}
				}},
			expected: []TextRange{{1, 10}},
		},
		"full overlap inverted": {
			input: []logFileProcessor{
				func(_ []byte) []TextRange {
					return []TextRange{{1, 10}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{2, 4}}
				}},
			expected: []TextRange{{1, 10}},
		},
		"test many ranges": {
			input: []logFileProcessor{
				func(_ []byte) []TextRange {
					return []TextRange{{16067, 16421}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{19659, 20140}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{16002, 16359}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{19559, 19951}}
				},
				func(_ []byte) []TextRange {
					return []TextRange{{59421, 59556}}
				}},
			expected: []TextRange{{16002, 16421}, {19559, 20140}, {59421, 59556}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.expected, Combine(nil, test.input...))
		})
	}
}

func TestReadLogFile(t *testing.T) {
	m := &Msiexec{}

	tests := map[string]struct {
		input    string
		expected string
	}{
		"Wix built-in failure mode": {
			input: "wixfailwhendeferred.log",
			expected: `--- 1547:2038
Action ended 2:10:53: LaunchConditions. Return value 1.
Action start 2:10:53: ValidateProductID.
Action ended 2:10:53: ValidateProductID. Return value 1.
Action start 2:10:53: WixRemoveFoldersEx.
WixRemoveFoldersEx:  Error 0x80070057: Missing folder property: dd_PROJECTLOCATION_0 for row: RemoveFolderEx
CustomAction WixRemoveFoldersEx returned actual error code 1603 but will be translated to success due to continue marking
Action ended 2:10:53: WixRemoveFoldersEx. Return value 1.

--- 6770:7391
Action start 2:10:55: PrepareDecompressPythonDistributions.
SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIB32B.tmp-\
SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.PrepareDecompressPythonDistributions
CA: 02:10:56: PrepareDecompressPythonDistributions. Could not set the progress bar size
CustomAction PrepareDecompressPythonDistributions returned actual error code 1603 but will be translated to success due to continue marking
Action ended 2:10:56: PrepareDecompressPythonDistributions. Return value 1.

--- 16002:16421
CA: 02:11:30: AddUser. ddagentuser already exists, not creating
CA: 02:11:30: GetPreviousAgentUser. Could not find previous agent user: System.Exception: Agent user information is not in registry
   at Datadog.CustomActions.InstallStateCustomActions.GetPreviousAgentUser(ISession session, IRegistryServices registryServices, INativeMethods nativeMethods)
CA: 02:11:30: ConfigureUser. Resetting ddagentuser password.

--- 19559:20140
CA: 02:11:36: ConfigureServiceUsers. Configuring services with account WIN-ST17FJ32SOG\ddagentuser
CA: 02:11:36: GetPreviousAgentUser. Could not find previous agent user: System.Exception: Agent user information is not in registry
   at Datadog.CustomActions.InstallStateCustomActions.GetPreviousAgentUser(ISession session, IRegistryServices registryServices, INativeMethods nativeMethods)
CA: 02:11:36: UpdateAndLogAccessControl. datadog-process-agent current ACLs: D:(A;;CCLCSWLOCRRC;;;IU)(A;;CCLCSWLOCRRC;;;SU)(A;;CCLCSWRPWPDTLOCRRC;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)

--- 24843:25383
CA(ddprocmon): DriverInstall:  Done with create() 0
CA(ddprocmon): DriverInstall:  done installing services
SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI593F.tmp-\
SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.StartDDServices
CustomAction WixFailWhenDeferred returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)
Action ended 2:11:49: InstallFinalize. Return value 3.

`,
		},
		"Missing password for DC": {
			input: "missing_password_for_dc.log",
			expected: `--- 3625:5242
SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.ProcessDdAgentUserCredentials
CA: 01:49:43: LookupAccountWithExtendedDomainSyntax. User not found, trying again with fixed domain part: \toto
CA: 01:49:43: ProcessDdAgentUserCredentials. User toto doesn't exist.
CA: 01:49:43: ProcessDdAgentUserCredentials. domain part is empty, using default
CA: 01:49:43: ProcessDdAgentUserCredentials. Installing with DDAGENTUSER_PROCESSED_NAME=toto and DDAGENTUSER_PROCESSED_DOMAIN=datadoghq-qa-labs.local
CA: 01:49:43: HandleProcessDdAgentUserCredentialsException. Error processing ddAgentUser credentials: Datadog.CustomActions.InvalidAgentUserConfigurationException: A password was not provided. A password is a required when installing on Domain Controllers.
   at Datadog.CustomActions.ProcessUserCustomActions.ProcessDdAgentUserCredentials(Boolean calledFromUIControl)
MSI (s) (C8!50) [01:49:43:906]: Product: Datadog Agent -- A password was not provided. A password is a required when installing on Domain Controllers.

A password was not provided. A password is a required when installing on Domain Controllers.
CustomAction ProcessDdAgentUserCredentials returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)
Action ended 1:49:43: ProcessDdAgentUserCredentials. Return value 3.
Action ended 1:49:43: INSTALL. Return value 3.
Property(S): UpgradeCode = {0C50421B-AEFB-4F15-A809-7AF256D608A5}
Property(S): NETFRAMEWORK45 = #528449
Property(S): WIXUI_EXITDIALOGOPTIONALCHECKBOX = 1

`,
		},
		"File in use": {
			input: "file_in_use.log",
			expected: `--- 3557:3890
Action start 1:45:18: InstallValidate.
Info 1603. The file C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe is being held in use by the following process: Name: process-agent, Id: 4704, Window Title: '(not determined yet)'. Close that application and retry.
Action ended 1:45:21: InstallValidate. Return value 1.

--- 5653:5849
SFXCA: Binding to CLR version v4.0.30319
Calling custom action CustomActions!Datadog.CustomActions.PrerequisitesCustomActions.EnsureAdminCaller
Action ended 1:45:22: RunAsAdmin. Return value 1.

--- 5983:6218
SFXCA: Binding to CLR version v4.0.30319
Calling custom action CustomActions!Datadog.CustomActions.InstallStateCustomActions.ReadInstallState
CA: 01:45:23: RegistryProperty. Found DDAGENTUSER_NAME in registry DDOG-HQ-QA-LABS\ddGmsa$

--- 7338:7520
SFXCA: Binding to CLR version v4.0.30319
Calling custom action CustomActions!Datadog.CustomActions.ConfigCustomActions.ReadConfig
Action ended 1:45:25: ReadConfig. Return value 1.

--- 7627:7960
Action start 1:45:25: InstallValidate.
Info 1603. The file C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe is being held in use by the following process: Name: process-agent, Id: 4704, Window Title: '(not determined yet)'. Close that application and retry.
Action ended 1:45:27: InstallValidate. Return value 1.

--- 12929:13180
SFXCA: Binding to CLR version v4.0.30319
Calling custom action CustomActions!Datadog.CustomActions.Rollback.RestoreDaclRollbackCustomAction.DoRollback
CA: 01:45:35: DoRollback. Resetting inheritance flag on "C:\Program Files\Datadog\Datadog Agent\"

--- 13273:13484
SFXCA: Binding to CLR version v4.0.30319
Calling custom action CustomActions!Datadog.CustomActions.ServiceCustomAction.StopDDServices
CA: 01:45:39: StopDDServices. Service datadog-system-probe status: Stopped

--- 14824:15093
SFXCA: Binding to CLR version v4.0.30319
Calling custom action CustomActions!Datadog.CustomActions.ConfigureUserCustomActions.UninstallUser
CA: 01:45:42: UninstallUser. Removing file access for DDOG-HQ-QA-LABS\ddGmsa$ (S-1-5-21-3647231507-2031390810-2876811253-1605)

--- 464565:465186
Action start 1:46:16: PrepareDecompressPythonDistributions.
SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI58A1.tmp-\
SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.PrepareDecompressPythonDistributions
CA: 01:46:17: PrepareDecompressPythonDistributions. Could not set the progress bar size
CustomAction PrepareDecompressPythonDistributions returned actual error code 1603 but will be translated to success due to continue marking
Action ended 1:46:17: PrepareDecompressPythonDistributions. Return value 1.

`,
		},
		"Invalid credentials": {
			input: "invalid_credentials.log",
			expected: `--- 7263:7883
Action start 1:50:19: PrepareDecompressPythonDistributions.
SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSID56.tmp-\
SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.PrepareDecompressPythonDistributions
CA: 01:50:19: PrepareDecompressPythonDistributions. Could not set the progress bar size
CustomAction PrepareDecompressPythonDistributions returned actual error code 1603 but will be translated to success due to continue marking
Action ended 1:50:19: PrepareDecompressPythonDistributions. Return value 1.

--- 25259:26729
CA: 01:50:48: StoreAgentUserInRegistry. Storing installedUser=ddagentuser
CA(ddnpm): DriverInstall:  Initialized
CA(ddnpm): DriverInstall:  Installing services
CA(ddnpm): DriverInstall:  installing service
CA(ddnpm): DriverInstall:  serviceDef::create()
CA(ddnpm): DriverInstall:  Failed to CreateService 1073
CA(ddnpm): DriverInstall:  Service exists, verifying
CA(ddnpm): DriverInstall:  Updated path for existing service
CA(ddnpm): DriverInstall:  done installing services
CA(ddapm): DriverInstall:  Initialized
CA(ddapm): DriverInstall:  Installing services
CA(ddapm): DriverInstall:  installing service
CA(ddapm): DriverInstall:  serviceDef::create()
CA(ddapm): DriverInstall:  Failed to CreateService 1073
CA(ddapm): DriverInstall:  Service exists, verifying
CA(ddapm): DriverInstall:  Updated path for existing service
CA(ddapm): DriverInstall:  done installing services
CA(ddprocmon): DriverInstall:  Initialized
CA(ddprocmon): DriverInstall:  Installing services
CA(ddprocmon): DriverInstall:  installing service
CA(ddprocmon): DriverInstall:  serviceDef::create()
CA(ddprocmon): DriverInstall:  Failed to CreateService 1073
CA(ddprocmon): DriverInstall:  Service exists, verifying
CA(ddprocmon): DriverInstall:  Updated path for existing service
CA(ddprocmon): DriverInstall:  done installing services
SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI7FD3.tmp-\
SFXCA: Binding to CLR version v4.0.30319

--- 26730:27404
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.StartDDServices
CA: 01:50:49: StartDDServices. Failed to start services: System.InvalidOperationException: Cannot start service datadogagent on computer '.'. ---> System.ComponentModel.Win32Exception: The service did not start due to a logon failure
   --- End of inner exception stack trace ---
   at System.ServiceProcess.ServiceController.Start(String args)
   at Datadog.CustomActions.Native.ServiceController.StartService(String serviceName, TimeSpan timeout)
   at Datadog.CustomActions.ServiceCustomAction.StartDDServices()
Action ended 1:50:49: InstallFinalize. Return value 1.

`,
		},
		"Service marked for deletion": {
			input: "service_marked_for_deletion.log",
			expected: `--- 1546:2037
Action ended 6:11:36: LaunchConditions. Return value 1.
Action start 6:11:36: ValidateProductID.
Action ended 6:11:36: ValidateProductID. Return value 1.
Action start 6:11:36: WixRemoveFoldersEx.
WixRemoveFoldersEx:  Error 0x80070057: Missing folder property: dd_PROJECTLOCATION_0 for row: RemoveFolderEx
CustomAction WixRemoveFoldersEx returned actual error code 1603 but will be translated to success due to continue marking
Action ended 6:11:36: WixRemoveFoldersEx. Return value 1.

--- 6876:7497
Action start 6:11:38: PrepareDecompressPythonDistributions.
SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI7CB7.tmp-\
SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.PrepareDecompressPythonDistributions
CA: 06:11:39: PrepareDecompressPythonDistributions. Could not set the progress bar size
CustomAction PrepareDecompressPythonDistributions returned actual error code 1603 but will be translated to success due to continue marking
Action ended 6:11:39: PrepareDecompressPythonDistributions. Return value 1.

--- 16762:17181
CA: 06:12:16: AddUser. ddagentuser already exists, not creating
CA: 06:12:16: GetPreviousAgentUser. Could not find previous agent user: System.Exception: Agent user information is not in registry
   at Datadog.CustomActions.InstallStateCustomActions.GetPreviousAgentUser(ISession session, IRegistryServices registryServices, INativeMethods nativeMethods)
CA: 06:12:16: ConfigureUser. Resetting ddagentuser password.

--- 19832:20285
  }
]
MSI (s) (B4:24) [06:12:21:764]: Product: Datadog Agent -- Error 1923. Service 'Datadog Agent' (datadogagent) could not be installed. Verify that you have sufficient privileges to install system services.

Error 1923. Service 'Datadog Agent' (datadogagent) could not be installed. Verify that you have sufficient privileges to install system services.
SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI2787.tmp-\

--- 20286:20851
SFXCA: Binding to CLR version v4.0.30319
Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.EnsureNpmServiceDependency
ExecServiceConfig:  Error 0x80070430: Cannot change service configuration. Error: The specified service has been marked for deletion.

ExecServiceConfig:  Error 0x80070430: Failed to configure service: datadogagent
CustomAction ExecServiceConfig returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)
Action ended 6:12:22: InstallFinalize. Return value 3.

`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			file, err := logFilesFS.Open("testdata/" + test.input)
			require.NoError(t, err)
			got, err := m.processLogFile(file)
			require.NoError(t, err)
			resultString := strings.ReplaceAll(string(got), "\r", "")
			require.Equal(t, test.expected, resultString)
		})
	}
}
