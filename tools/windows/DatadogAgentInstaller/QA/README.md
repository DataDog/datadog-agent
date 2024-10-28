# Semi-automated QA tests for the Windows installer

This folder contains a set of PowerShell script to help run QA tests for the Windows installer. This is **not** intended as a replacement / substitution for e2e tests but are merely a collection of scripts to quickly run installer tests on a given Hyper-V VM.

# Motivation

This set of script was written with the goal of simplifying repetitive manual QA tests on the installer.

It was designed with simplicity in mind, allowing to quickly and easily share a reproducible scenario to a coworker and being able to run the scenario without any prerequisites apart from a Hyper-V VM.

# Running the test suite

Several parameters are required to run the test suite:
- `vmName`: The name of the VM as it appears in Hyper-V
- `vmUserName`: The name of an existing user in the guest Hyper-V VM
- `vmUserPassword`: The password of that user
- `vmInitialSnapshotName`: The snapshot that the qa script will use as a basis to create its own. It must exists.
- `qaSessionName`: A description of the QA session, it's used to create snaphots in the VM. Useful for running multiple QA sessions in parallel.
- `qaArtifactsPath`: A path to a folder where all the MSIs will be sourced.

Here's an example command:
```
PS C:\dev\datadog-agent> & .\tools\windows\DatadogAgentInstaller\QA\qa_installer.ps1 -vmName "Windows Server 2022 FR" -vmUserName "win-3f423u4i7ug\administrateur" -vmUserPassword "********" -vmInitialSnapshotName "Windows Server 2022 FR - (27/07/2023 - 12:06:27)" -qaSessionname "Non-canonical permission issue QA" -qaArtifactsPath "C:\dev\QA\artifacts"
```

# Test suites & cases
The test suites & test cases are to be placed in the "test suites" folder. They will be picked up automatically by the test framework.

Test cases are organized in suites, and the test framework executes one test suite after another.

Each test suite has its own Hyper-V snapshot, and by default it will re-use it for faster iteration time.
This allows every tests to run against a known checkpoint.

An additional benefit is the use of `BeforeTest`, which is a code block that will be executed in the guest VM and captured in the snapshot. This is useful for example to pre-install an Agent version that will be used in every test in an upgrade test suite for example.

## How to write a new test case
To write a test, either pick an existing `Suite` or create a new one:
```
Suite "My new test suite" {

}
```

Two keywords are allowed in a `Suite`:
- `BeforeTest`
- `Case`

`BeforeTest` allows specifying a code block that will be executed before each tests.

`Case` creates a new test case. Test cases are executed sequentially and in the order they were defined.

Adding a test case:
```
Suite "My new test suite" {
    Case "test case 1" {

    }
}
```

Two keywors are allowed in a `Case`:
- `Require`
- `Test`

`Require` allows specifying an array of artifacts needed for the test. The artifacts are gathered within a suite, and duplicates are removed. They will be transferred from the "qa artifacts path" to the guest VM ahead of time in the "Suite"'s snapshot, to minimize the time it takes to start the test.

`Case` allows defining the code to be run for a test. The code will run in the context of the guest VM. All the code defined in the `common_test_code.ps1` will be callable.

A simple test case can be to install the Agent and check that the "datadogagent" service is running:

```
Suite "My new test suite" {
    Case "test case 1" {
        Require @(
            "datadog-agent-7.49.0-1.x86_64.msi"
        )
        Test {
            $installResult = Install-MSI -installerName datadog-agent-7.49.0-1.x86_64.msi
            Assert_InstallSuccess $installResult
        }
    }
}
```

The object `$installResult` returned by the `Install-MSI` function contains both the result code of `msiexec` and the install logs. It's possible to do all sort of cool things with the install logs, like logging every instance of a running `CustomAction` with 3 lines of context before and after:

```
Suite "My new test suite" {
    Case "test case 1" {
        Require @(
            "datadog-agent-7.49.0-1.x86_64.msi"
        )
        Test {
            $installResult = Install-MSI -installerName datadog-agent-7.49.0-1.x86_64.msi
            Assert_InstallSuccess $installResult
            $installResult.InstallLogs | Selec-String -Pattern "Running CustomAction" -Context 3 | Write-Host
        }
    }
}
```

This will print:
```
[09/28/23 13:46:14] == datadog-agent-7.49.0-1.x86_64.msi with args 
[09/28/23 13:47:15] == Running scenario 'test case 1'
[09/28/23 13:47:15] == Installing datadog-agent-7.49.0-1.x86_64.msi with args
  Action start 13:47:16: WixSharp_InitRuntime_Action.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIE0A5.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action WixSharp!WixSharp.ManagedProjectActions.WixSharp_InitRuntime_Action
  Action ended 13:47:17: WixSharp_InitRuntime_Action. Return value 1.
  Action start 13:47:17: AppSearch.
  Action ended 13:47:17: AppSearch. Return value 1.
  Action start 13:47:17: ReadInstallState.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIE3A4.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.InstallStateCustomActions.ReadInstallState
  CA: 13:47:18: RegistryProperty. Found DDAGENTUSER_NAME in registry win-3f423u4i7ug\ddagentuser
  CA: 13:47:18: RegistryProperty. Found PROJECTLOCATION in registry C:\Program Files\Datadog\Datadog Agent\
  CA: 13:47:18: RegistryProperty. Found APPLICATIONDATADIRECTORY in registry C:\ProgramData\Datadog\
  Action start 13:47:25: ReadConfig.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI1EC.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ConfigCustomActions.ReadConfig
  Action ended 13:47:27: ReadConfig. Return value 1.
  Action start 13:47:27: MigrateFeatureStates.
  Action ended 13:47:27: MigrateFeatureStates. Return value 1.
  Action start 13:47:31: ProcessDdAgentUserCredentials.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI190F.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ProcessUserCustomActions.ProcessDdAgentUserCredentials
  CA: 13:47:32: ProcessDdAgentUserCredentials. Found ddagentuser in WIN-3F423U4I7UG as SidTypeUser
  CA: 13:47:32: ProcessDdAgentUserCredentials. "WIN-3F423U4I7UG\ddagentuser" (S-1-5-21-2624507216-3994333568-3321347143-1000, SidTypeUser) is a local account
  CA: 13:47:32: ProcessDdAgentUserCredentials. Installing with DDAGENTUSER_PROCESSED_NAME=ddagentuser and DDAGENTUSER_PROCESSED_DOMAIN=WIN-3F423U4I7UG
  Action start 13:48:33: PrepareDecompressPythonDistributions.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIBC7.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.PythonDistributionCustomAction.PrepareDecompressPythonDistributions
  CA: 13:48:34: PrepareDecompressPythonDistributions. Could not set the progress bar size
  CustomAction PrepareDecompressPythonDistributions returned actual error code 1603 but will be translated to success due to continue marking
  Action ended 13:48:34: PrepareDecompressPythonDistributions. Return value 1.
  Action start 13:48:34: InstallFinalize.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI1187.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ServiceCustomAction.StopDDServices
  CA: 13:48:35: StopDDServices. Stopping service datadog-system-probe
  CA: 13:48:35: StopDDServices. Service datadog-system-probe status: Stopped
  CA: 13:48:35: StopDDServices. Service ddnpm not found
  CA: 13:48:35: StopDDServices. Service datadogagent status: Stopped
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI282D.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.PatchInstallerCustomAction.Patch
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI2ED6.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.PythonDistributionCustomAction.DecompressPythonDistributions
  CA: 13:48:42: DecompressPythonDistribution. C:\Program Files\Datadog\Datadog Agent\embedded2 not found, skip deletion.
  CA: 13:48:42: DecompressPythonDistribution. C:\Program Files\Datadog\Datadog Agent\embedded2.COMPRESSED not found, skipping decompression.
  CA: 13:48:42: DecompressPythonDistribution. C:\Program Files\Datadog\Datadog Agent\embedded3 not found, skip deletion.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIBE6F.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ConfigureUserCustomActions.ConfigureUser
  CA: 13:50:25: AddUser. ddagentuser already exists, not creating
  CA: 13:50:25: GetPreviousAgentUser. Found agent user information in registry win-3f423u4i7ug\ddagentuser
  CA: 13:50:25: GetPreviousAgentUser. Found previous agent user win-3f423u4i7ug\ddagentuser (S-1-5-21-2624507216-3994333568-3321347143-1000)
  ]
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSID8A0.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ServiceCustomAction.EnsureNpmServiceDependency
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIDD27.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ServiceCustomAction.ConfigureServiceUsers
  CA: 13:50:32: ConfigureServiceUsers. Configuring services with account WIN-3F423U4I7UG\ddagentuser
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIDFB8.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.Interfaces.InstallInfoCustomActions.WriteInstallInfo
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIE68F.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.InstallStateCustomActions.WriteInstallState
  CA: 13:50:35: WriteInstallState. Storing installedDomain=WIN-3F423U4I7UG
  CA: 13:50:35: WriteInstallState. Storing installedUser=ddagentuser
  CA: CADriverInstall:  Installing services
  CA: CADriverInstall:  done installing services
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSIE9CD.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ServiceCustomAction.StartDDServices
  CustomAction WixFailWhenDeferred returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)
  Action ended 13:50:58: InstallFinalize. Return value 3.
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI4175.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ServiceCustomAction.StartDDServicesRollback
  CA: 13:50:58: StopDDServices. Stopping service datadog-system-probe
  CA: 13:50:58: StopDDServices. Service datadog-system-probe status: Stopped
  CA: 13:50:58: StopDDServices. Stopping service ddnpm
  CA: 13:51:05: StopDDServices. Service datadogagent status: Running
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI5F30.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.ConfigureUserCustomActions.ConfigureUserRollback
  CA: 13:51:06: Load. Loading rollback info: [
    {
      "$type": "FilePermissionRollbackData",
  CA: 13:51:09: Restore. C:\ProgramData\Datadog\ new ACLs: O:SYG:SYD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;S-1-5-21-2624507216-3994333568-3321347143-1000)
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI6D99.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.CleanUpFilesCustomAction.CleanupFiles
  CA: 13:51:10: CleanupFiles. C:\Program Files\Datadog\Datadog Agent\embedded2 not found, skip deletion.
  CA: 13:51:10: CleanupFiles. Deleting directory "C:\Program Files\Datadog\Datadog Agent\embedded3"
  CA: 13:51:12: CleanupFiles. Deleting file "C:\ProgramData\Datadog\install_info"
  CA: 13:51:12: CleanupFiles. Deleting file "C:\ProgramData\Datadog\auth_token"
  SFXCA: Extracting custom action to temporary directory: C:\Windows\Installer\MSI7B69.tmp-\
  SFXCA: Binding to CLR version v4.0.30319
> Calling custom action CustomActions!Datadog.CustomActions.Telemetry.ReportFailure
  CA: 13:51:13: Report. Sending installation telemetry
  CA: 13:51:13: ReportTelemetry. API key empty, not reporting telemetry
  CA: DoRollback:  Initialized.
[09/28/23 13:52:21] ❌ Expected installation to succeed, but it failed with code 1603
    + CategoryInfo          : NotSpecified: (:) [Write-Error], WriteErrorException
    + FullyQualifiedErrorId : Microsoft.PowerShell.Commands.WriteErrorException,report_err
    + PSComputerName        : Windows Server 2022 FR
```

A test will fail if any error are thrown within its scope of execution (except of course with the usage of `-ErrorAction` to suppress the error). Some default assertions are provided, such as:
- `Assert_DaclAutoInherited_Flag_Set`: Asserts that a directory exists and it has the auto-inherited flag set.
- `Assert_DaclAutoInherited_Flag_NotSet`: The opposite of the previous assertion. Provided for convenience. The directory must still exist.
- `Assert_DirectoryExists`: Asserts that the directory exist.
- `Assert_DirectoryNotExists`: Asserts that the directory doesn't exist.
- `Assert_InstallSuccess`: Checks that the result of `Install-MSI` or `Uninstall-MSI` is 0, and if not, reports an error and prints the logs in the console.
- `Assert_InstallFinishedWithCode`: Asserts that the install finished with a specific error code (typically `1603`).
