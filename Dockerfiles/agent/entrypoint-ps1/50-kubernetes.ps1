
if (-not (Test-Path env:KUBERNETES)) { 
    exit 0
}

# Set a default config for Kubernetes if found
# Don't override datadog.yaml if it exists
if (-not (Test-Path C:\ProgramData\Datadog\datadog.yaml)) { 
       cp C:\ProgramData\Datadog\datadog-kubernetes.yaml C:\ProgramData\Datadog\datadog.yaml
}
