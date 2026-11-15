
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
    cp C:\ProgramData\Datadog\datadog-docker.yaml C:\ProgramData\Datadog\datadog.yaml
}
