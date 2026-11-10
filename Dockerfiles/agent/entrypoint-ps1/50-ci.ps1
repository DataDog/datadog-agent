
if (-not (Test-Path env:DD_INSIDE_CI)) {
    exit 0
}

# Set a default config for CI environments
# Don't override datadog.yaml if it exists
if (-not (Test-Path C:\ProgramData\Datadog\datadog.yaml)) {
   cp C:\ProgramData\Datadog\datadog-ci.yaml C:\ProgramData\Datadog\datadog.yaml
}

# Remove all default checks
Get-ChildItem C:\ProgramData\Datadog\conf.d -Include *.yaml.default -Recurse | Remove-Item
