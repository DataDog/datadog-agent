
# Disable infra checks that would report metrics from the container (instead of the host),
# if they're not already disabled

$defaultChecks = @(
    "disk",
    "network",
    "winproc",
    "file_handle",
    # TODO: Conditionally enable the IO check if the host drives are mounted in the container
    "io"
)

ForEach ($defaultCheck in $defaultChecks) {
    $confPath = "C:\ProgramData\Datadog\conf.d\$defaultCheck.d\conf.yaml.default"
    if (Test-Path $confPath) {
        Remove-Item $confPath
    }
}
