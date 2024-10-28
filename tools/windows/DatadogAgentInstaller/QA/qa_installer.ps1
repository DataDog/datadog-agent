# using Module to import PowerShell classes
using Module ".\test_framework.psm1"

param (
    [string]$vmName,
    [string]$vmUserName,
    [string]$vmUserPassword,
    [string]$vmInitialSnapshotName,
    [string]$qaSessionname,
    [string]$qaArtifactsPath
)

$myTestContext = [TestContext]::new(
    $vmName,
    $vmUserName,
    $vmUserPassword,
    $vmInitialSnapshotName,
    $qaSessionname,
    $qaArtifactsPath
)
$myTestContext.Prepare_Host_Machine()
$myTestContext.Prepare_Target_Machine()

Remove-Variable -Name testSuites -Scope Global -ErrorAction SilentlyContinue
Remove-Variable -Name currentSuite -Scope Global -ErrorAction SilentlyContinue
Remove-Variable -Name currentCase -Scope Global -ErrorAction SilentlyContinue
# Since we removed the variables to have a clean env, reload the module.
Remove-Module "test_framework"
Import-Module $PSScriptRoot\test_framework.psm1

Get-ChildItem -Path "$PSScriptRoot\test suites" -Recurse -Filter "*.ps1" | ForEach-Object {
    Write-Host "Loading $($_.FullName)"
    & $_.FullName
}
$global:testSuites | ForEach-Object {
    # it's safer to clear the snaphot ($true for the second param) by default
    # otherwise it will reuse the artifacts captured in the snapshot.
    $_.Execute($myTestContext, $true)
}

Remove-Variable -Name testSuites -Scope Global -ErrorAction SilentlyContinue
Remove-Variable -Name currentSuite -Scope Global -ErrorAction SilentlyContinue
Remove-Variable -Name currentCase -Scope Global -ErrorAction SilentlyContinue
Remove-Module "test_framework"