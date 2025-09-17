# Build the Agent packages

## Windows Docker image (Windows host only, recommended)

### Prerequisites
To build on Windows, [Docker Desktop](https://docs.docker.com/docker-for-windows/install/) must be installed and configured to use Windows containers.

Start a Powershell prompt and navigate to your local clone of the `datadog-agent` repo.

 Run the following command:

```powershell
docker run -v "$(Get-Location):c:\mnt" -e OMNIBUS_TARGET=main -e MAJOR_VERSION=7 -e TARGET_ARCH=x64 datadog/agent-buildimages-windows_x64:1809 powershell -C "c:\mnt\tasks\winbuildscripts\Build-AgentPackages.ps1 -BuildOutOfSource 1 -InstallDeps 1 -CheckGoVersion 1"
```

Downloading the Docker image may take some time in the first run.

Alternatively here's a small Powershell script to facilitate using the docker image:
```powershell
param (
   [int]$MAJOR_VERSION=7,
   $TARGET_ARCH="x64",
   [bool]$RM_CONTAINER=$true,
   [bool]$DEBUG=$false
)

$cmd = "docker run"
if ($RM_CONTAINER) {
    $cmd += " --rm "
}
$opts = "-e OMNIBUS_TARGET=main -e MAJOR_VERSION=$MAJOR_VERSION -e TARGET_ARCH=$TARGET_ARCH"
if ($DEBUG) {
    $opts += " -e DEBUG_CUSTOMACTION=yes "
}
$cmd += " -m 8192M -v ""$(Get-Location):c:\mnt"" $opts datadog/agent-buildimages-windows_x64:1809 powershell -C ""c:\mnt\tasks\winbuildscripts\Build-AgentPackages.ps1 -BuildOutOfSource 1 -InstallDeps 1 -CheckGoVersion 1"""
Write-Host $cmd
Invoke-Expression -Command $cmd
```

If the build succeeds, the build artifacts can be found under `omnibus\pkg` in the repo.
