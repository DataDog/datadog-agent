param(
    [Parameter(Mandatory=$True)]
    [string]$user,

    [Parameter(Mandatory=$True)]
    [string]$executable,

    [Parameter(Mandatory=$True)]
    [string]$password,

    [Parameter(Mandatory=$True)]
    [string]$payload
)

$ErrorActionPreference = "Stop"

$cmd  = New-Object System.Diagnostics.ProcessStartInfo;

$cmd.FileName = $executable
$cmd.RedirectStandardOutput = $true
$cmd.RedirectStandardError = $true
$cmd.RedirectStandardInput = $true
$cmd.UseShellExecute = $false
$cmd.UserName = $user
$cmd.Password = ConvertTo-SecureString $password -AsPlainText -Force

"Creating new Process with $($executable)"
$process = [System.Diagnostics.Process]::Start($cmd);

"Waiting a second for the process to be up and running"
Start-Sleep -s 1

"Writing the payload to Stdin"
$process.StandardInput.WriteLine($payload)
$process.StandardInput.Close()

"Waiting a second so the process can fetch the secrets"
Start-Sleep -s 1

"stdout:"
$process.StandardOutput.ReadToEnd()
if ($process.StandardOutErr) {
    "stderr:"
    $process.StandardOutErr.ReadToEnd()
} else {
    "stderr: None"
}
"exit code:"
$process.ExitCode
