$ErrorActionPreference = "Stop"

$cmd  = New-Object System.Diagnostics.ProcessStartInfo;

$cmd.FileName = $args[0]
$cmd.RedirectStandardOutput = $true
$cmd.RedirectStandardError = $true
$cmd.RedirectStandardInput = $true
$cmd.UseShellExecute = $false
$cmd.UserName = "datadog_secretuser"
$cmd.Password = ConvertTo-SecureString $args[1] -AsPlainText -Force

"Creating new Process with $($args[0])"
$process = [System.Diagnostics.Process]::Start($cmd);

"Waiting a second for the process to be up and running"
Start-Sleep -s 1

"Writing the payload to Stdin"
$process.StandardInput.WriteLine($args[2])
$process.StandardInput.Close()

"Waiting a seconds so the process can fetch the secrets"
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
