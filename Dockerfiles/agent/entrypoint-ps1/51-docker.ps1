
# We enable docker if either:
#   - we detect the DOCKER_HOST envvar, overriding the default socket location
#     (in that case, we trust the user wants docker integration and don't check existence)
#   - we find the docker socket at it's default location
if (-Not (Test-Path env:DOCKER_HOST) -And -Not (Test-Path \\.\pipe\docker_engine)) { 
    exit 0
}

# Set a config for vanilla Docker if no orchestrator was detected
# by the 50-* scripts
# Don't override datadog.yaml if it exists
if (-not (Test-Path C:\ProgramData\Datadog\datadog.yaml)) { 
    Write-Output "Autodiscovery enabled for Kubernetes"
    cp C:\ProgramData\Datadog\datadog-docker.yaml C:\ProgramData\Datadog\datadog.yaml
}

# Enable the docker corecheck
if (-Not (Test-Path C:\ProgramData\Datadog\conf.d\docker.d\conf.yaml.default) -And (Test-Path C:\ProgramData\Datadog\conf.d\docker.d\conf.yaml.example)) { 
    mv C:\ProgramData\Datadog\conf.d\docker.d\conf.yaml.example C:\ProgramData\Datadog\conf.d\docker.d\conf.yaml.default
}
