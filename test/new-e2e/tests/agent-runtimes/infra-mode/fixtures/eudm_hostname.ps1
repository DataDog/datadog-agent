# Read the VM's actual identity independently of the Agent.
$name = $env:COMPUTERNAME.ToLowerInvariant()
$serial = (Get-CimInstance Win32_BIOS).SerialNumber.Trim().ToLowerInvariant()
$serial = ($serial -replace '[^a-z0-9]+', '-').Trim('-')
if ([string]::IsNullOrEmpty($serial)) {
    throw 'BIOS serial is unavailable'
}
Write-Output ($name + '-' + $serial)
