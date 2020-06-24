
if (-not (Test-Path env:KUBERNETES)) { 
    exit 0
}

# Set a default config for Kubernetes if found
# Don't override datadog.yaml if it exists
if (-not (Test-Path C:\ProgramData\Datadog\datadog.yaml)) { 
       Write-Output "Autodiscovery enabled for Kubernetes"
   if (Test-Path \\.\pipe\docker_engine) { 
       cp C:\ProgramData\Datadog\datadog-k8s-docker.yaml C:\ProgramData\Datadog\datadog.yaml
   } else {
       cp C:\ProgramData\Datadog\datadog-kubernetes.yaml C:\ProgramData\Datadog\datadog.yaml
   }
}

# Enable kubernetes integrations (don't fail if integration absent)
if (-not (Test-Path C:\ProgramData\Datadog\conf.d\kubelet.d\conf.yaml.default)) {
    cp C:\ProgramData\Datadog\conf.d\kubelet.d\conf.yaml.example C:\ProgramData\Datadog\conf.d\kubelet.d\conf.yaml.default 
}
