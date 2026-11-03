# Test-only instrumentation of a downloaded MSI. Production custom actions and their
# relative ordering are preserved; checkpoints observe SCM settings during execution.
param(
    [Parameter(Mandatory)][string]$MsiPath,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [ValidateSet('old', 'new')][string]$Role,
    [string]$FailAfter = ''
)
$ErrorActionPreference = 'Stop'
$installer = New-Object -ComObject WindowsInstaller.Installer
$database = $installer.OpenDatabase($MsiPath, 1)
function Execute-Sql([string]$sql) {
    $view = $database.OpenView($sql)
    try { $view.Execute() } finally { $view.Close() }
}
function Read-Rows([string]$sql) {
    $view = $database.OpenView($sql)
    try {
        $view.Execute()
        while ($record = $view.Fetch()) { $record }
    } finally { $view.Close() }
}
function Quote-Sql([string]$value) { "'" + $value.Replace("'", "''") + "'" }

# Leave room for observation and failure actions without changing relative ordering.
$sequences = @{}
foreach ($row in (Read-Rows 'SELECT `Action`, `Sequence` FROM `InstallExecuteSequence` ORDER BY `Sequence` DESC')) {
    $name = $row.StringData(1)
    $sequence = $row.IntegerData(2)
    if ($sequence -le 0) { continue }
    $sequence *= 3
    if ($sequence -gt 32767) { throw 'MSI sequence cannot be expanded safely' }
    $sequences[$name] = $sequence
    Execute-Sql ('UPDATE `InstallExecuteSequence` SET `Sequence` = ' + $sequence + ' WHERE `Action` = ' + (Quote-Sql $name))
}

# Validate the compiled sequence, not just the WixSharp relative declarations.
if ($sequences['RemoveExistingProducts'] -le $sequences['InstallInitialize'] -or
    $sequences['RemoveExistingProducts'] -ge $sequences['ProcessComponents']) {
    throw 'Expected old-product removal immediately after transaction initialization'
}
if ($sequences.ContainsKey('InstallExecute') -and $sequences['InstallExecute'] -lt $sequences['RemoveExistingProducts']) {
    throw 'Unexpected early InstallExecute'
}
if ($sequences['PrepareServiceStartupRollback'] -ge $sequences['InstallInitialize'] -or
    $sequences['StopDDServicesRollback'] -ge $sequences['StopDDServices'] -or
    $sequences['StopDDServices'] -ge $sequences['UninstallUser']) {
    throw 'Unexpected service-protection sequence'
}
foreach ($row in (Read-Rows 'SELECT `Action`, `Source` FROM `CustomAction` WHERE `Type` = 51')) {
    if ($row.StringData(2) -in @('StopDDServices', 'StopDDServicesRollback')) {
        if ($sequences[$row.StringData(1)] -le $sequences['PrepareServiceStartupRollback']) {
            throw 'Snapshot identity must exist before custom-action data is captured'
        }
    }
}

$condition = if ($Role -eq 'old') { 'UPGRADINGPRODUCTCODE' } else { 'NOT Installed' }
$checkpoints = if ($Role -eq 'old') { @('StopDDServices', 'UninstallUser') } else { @('ConfigureUser', 'ConfigureServices', 'StartDDServices') }
foreach ($anchor in $checkpoints) {
    if (-not $sequences.ContainsKey($anchor)) { throw "Missing action $anchor" }
    $name = "TestStartupAfter$anchor"
    $output = Join-Path $OutputDirectory "$Role-$anchor.txt"
    $script = @"
Dim shell, fs, output, name, start, readError
Set shell = CreateObject("WScript.Shell")
Set fs = CreateObject("Scripting.FileSystemObject")
Set output = fs.CreateTextFile("$output", True)
For Each name In Array("datadogagent", "dd-procmgr-service", "Datadog Installer")
    On Error Resume Next
    start = shell.RegRead("HKLM\SYSTEM\CurrentControlSet\Services\" & name & "\Start")
    readError = Err.Number
    Err.Clear
    On Error GoTo 0
    If readError = &H80070002 Then
        start = "absent"
    ElseIf readError <> 0 Then
        Err.Raise readError
    End If
    output.WriteLine name & "=" & start
Next
output.Close
"@
    # Type 38: inline VBScript; 1024: deferred; 2048: no impersonation; 4096: 64-bit.
    Execute-Sql ('INSERT INTO `CustomAction` (`Action`, `Type`, `Target`) VALUES (' + (Quote-Sql $name) + ', 7206, ' + (Quote-Sql $script) + ')')
    Execute-Sql ('INSERT INTO `InstallExecuteSequence` (`Action`, `Condition`, `Sequence`) VALUES (' + (Quote-Sql $name) + ', ' + (Quote-Sql $condition) + ', ' + ($sequences[$anchor] + 1) + ')')
}
if ($FailAfter) {
    if (-not $checkpoints.Contains($FailAfter)) { throw "No checkpoint for failure at $FailAfter" }
    # Reuse WiX's real deferred failure action. Its new condition also applies in
    # the old product's nested uninstall, without relying on property propagation.
    $failure = @(Read-Rows 'SELECT `Action` FROM `CustomAction` WHERE `Target` = ''WixFailWhenDeferred''')
    if ($failure.Count -ne 1) { throw 'Expected exactly one WiX deferred failure action' }
    $name = $failure[0].StringData(1)
    Execute-Sql ('UPDATE `InstallExecuteSequence` SET `Condition` = ' + (Quote-Sql $condition) + ', `Sequence` = ' + ($sequences[$FailAfter] + 2) + ' WHERE `Action` = ' + (Quote-Sql $name))
}
# A modified package must have a new package code (ProductCode remains unchanged).
$summary = $database.SummaryInformation(1)
$summary.GetType().InvokeMember('Property', 'SetProperty', $null, $summary, @(9, [guid]::NewGuid().ToString('B').ToUpperInvariant())) | Out-Null
$summary.Persist()
$database.Commit()
