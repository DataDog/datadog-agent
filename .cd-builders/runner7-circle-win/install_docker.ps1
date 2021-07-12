$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

mkdir C:\Docker

# Docker CLI builds maintained by a Docker engineer
$dockerVersion = "19.03.3"
Invoke-WebRequest -Uri "https://github.com/StefanScherer/docker-cli-builder/releases/download/$dockerVersion/docker.exe" -OutFile "C:\Docker\docker.exe"

# Install manifest-tool
$manifestVersion = "v1.0.1"
Invoke-WebRequest -Uri "https://github.com/estesp/manifest-tool/releases/download/$manifestVersion/manifest-tool-windows-amd64.exe" -OutFile "C:\Docker\manifest-tool.exe"

# Install notary
$notaryVersion = "v0.6.1"
Invoke-WebRequest -Uri "https://github.com/theupdateframework/notary/releases/download/$notaryVersion/notary-Windows-amd64.exe" -OutFile "C:\Docker\notary.exe"

# Add Docker to path
setx PATH "$Env:Path;C:\Docker"
$Env:Path="$Env:Path;C:\Docker"
