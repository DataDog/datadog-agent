
# Disable infra checks that would report metrics from the container (instead of the host),
# if they're not already disabled.
#
# Environment variable overrides:
#   DD_WINDOWS_HOST_METRICS=true       - Keep ALL infra checks enabled
#   DD_WINDOWS_ENABLE_DISK_CHECK=true  - Keep the disk check enabled
#   DD_WINDOWS_ENABLE_NETWORK_CHECK=true - Keep the network check enabled
#   DD_WINDOWS_ENABLE_WINPROC_CHECK=true - Keep the winproc check enabled
#   DD_WINDOWS_ENABLE_FILE_HANDLE_CHECK=true - Keep the file_handle check enabled
#   DD_WINDOWS_ENABLE_IO_CHECK=true    - Keep the IO check enabled

if ($env:DD_WINDOWS_HOST_METRICS -eq "true") {
    Write-Output "DD_WINDOWS_HOST_METRICS is set, keeping all infra checks enabled"
    exit 0
}

$defaultChecks = @{
    "disk"        = "DD_WINDOWS_ENABLE_DISK_CHECK"
    "network"     = "DD_WINDOWS_ENABLE_NETWORK_CHECK"
    "winproc"     = "DD_WINDOWS_ENABLE_WINPROC_CHECK"
    "file_handle" = "DD_WINDOWS_ENABLE_FILE_HANDLE_CHECK"
    "io"          = "DD_WINDOWS_ENABLE_IO_CHECK"
}

ForEach ($entry in $defaultChecks.GetEnumerator()) {
    $checkName = $entry.Key
    $envVar = $entry.Value
    $confPath = "C:\ProgramData\Datadog\conf.d\$checkName.d\conf.yaml.default"

    $envValue = [System.Environment]::GetEnvironmentVariable($envVar)
    if ($envValue -eq "true") {
        Write-Output "$envVar is set, keeping $checkName check enabled"
        continue
    }

    if (Test-Path $confPath) {
        Remove-Item $confPath
    }
}
