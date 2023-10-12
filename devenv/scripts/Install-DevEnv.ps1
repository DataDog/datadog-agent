# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog
# (https://www.datadoghq.com/).
# Copyright 2019-present Datadog, Inc.

Write-Host
'=======================================================
         ____        __        ____
        / __ \____ _/ /_____ _/ __ \____  ____ _
       / / / / __ `/ __/ __ `/ / / / __ \/ __ `/
      / /_/ / /_/ / /_/ /_/ / /_/ / /_/ / /_/ /
     /_____/\__,_/\__/\__,_/_____/\____/\__, /
                                       /____/
=======================================================
                    * WinDog Setup *'

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Getting Chocolatey'
Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))

# Imports 'Update-SessionEnvironment' so we can reload env variables without restarting the process
Import-Module "$env:ChocolateyInstall\helpers\chocolateyInstaller.psm1" -Force;

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Getting git'
cinst -y git

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Getting 7zip'
cinst -y 7zip

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Installing CMake'
cinst -y cmake
[Environment]::SetEnvironmentVariable(
    "Path",
    [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";${env:ProgramFiles}\CMake\bin",
    [System.EnvironmentVariableTarget]::Machine)

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Installing Golang'

# TODO: Enable this when we can use Chocolatey again
#cinst -y golang --version 1.15.13

# Workaround for go 1.15.13 since it does not exist in Chocolatey
# taken from https://github.com/DataDog/datadog-agent-buildimages/blob/master/windows/install_go.ps1
# (workaround kept for later versions)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

Write-Host -ForegroundColor Green "Installing go 1.20.8"

$gozip = "https://dl.google.com/go/go1.20.8.windows-amd64.zip"
if ($Env:TARGET_ARCH -eq "x86") {
    $gozip = "https://dl.google.com/go/go1.20.8.windows-386.zip"
}

$out = 'c:\go.zip'
Write-Host -ForegroundColor Green "Downloading $gozip to $out"
(New-Object System.Net.WebClient).DownloadFile($gozip, $out)
Write-Host -ForegroundColor Green "Extracting $out to c:\"
Start-Process "7z" -ArgumentList 'x -oc:\ c:\go.zip' -Wait
Write-Host -ForegroundColor Green "Removing temporary file $out"
Remove-Item 'c:\go.zip'

[Environment]::SetEnvironmentVariable(
    "Path",
    [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";C:\go\bin",
    [System.EnvironmentVariableTarget]::Machine)

setx /m GOROOT c:\go
# End Go workaround

Write-Host -ForegroundColor Green "Installed go $ENV:GO_VERSION"

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Installing Python 3'
cinst -y python3

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Installing MINGW'
cinst -y mingw

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen '- Installing Make'
cinst -y make

$GoPath="C:\gopath"
$AgentPath="$GoPath\src\github.com\datadog\datadog-agent"
mkdir -Force $AgentPath

[Environment]::SetEnvironmentVariable(
    "Path",
    [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";$GoPath\bin;$AgentPath\rtloader\bin",
    [System.EnvironmentVariableTarget]::Machine)

setx /m GOPATH "$GoPath"

Write-Host -ForegroundColor Yellow -BackgroundColor DarkGreen ' * DONE *'
