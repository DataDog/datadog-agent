$ErrorActionPreference = "Stop"
Trap { Write-Host "Error in install.ps1: $_" }

if ("$env:WITH_JMX" -ne "false") {
    Invoke-WebRequest -OutFile jre-11.0.6.zip https://github.com/AdoptOpenJDK/openjdk11-binaries/releases/download/jdk-11.0.6%2B10/OpenJDK11U-jre_x64_windows_hotspot_11.0.6_10.zip
    Expand-Archive -Path jre-11.0.6.zip -DestinationPath C:/
    Remove-Item jre-11.0.6.zip
    Move-Item C:/jdk-11.0.6+10-jre/ C:/java
    # Add java to the path for jmxfetch
    setx /m PATH "$Env:Path;C:/java/bin"
    $Env:Path="$Env:Path;C:/java/bin"
}

Expand-Archive datadog-agent-7-latest.amd64.zip
Remove-Item datadog-agent-7-latest.amd64.zip

Get-ChildItem -Path datadog-agent-7-* | Rename-Item -NewName "Datadog Agent"

New-Item -ItemType directory -Path "C:/Program Files/Datadog"
Move-Item "Datadog Agent" "C:/Program Files/Datadog/"

New-Item -ItemType directory -Path 'C:/ProgramData/Datadog' 
Move-Item "C:/Program Files/Datadog/Datadog Agent/EXAMPLECONFSLOCATION" "C:/ProgramData/Datadog/conf.d"

# Register as a service
New-Service -Name "datadogagent" -StartupType "Manual" -BinaryPathName "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe"
New-Service -Name "datadog-process-agent" -StartupType "Manual" -BinaryPathName "C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe"
New-Service -Name "datadog-trace-agent" -StartupType "Manual" -BinaryPathName "C:\Program Files\Datadog\Datadog Agent\bin\agent\trace-agent.exe"

# Allow to run agent binaries as `agent`
setx /m PATH "$Env:Path;C:/Program Files/Datadog/Datadog Agent/bin;C:/Program Files/Datadog/Datadog Agent/bin/agent"
$Env:Path="$Env:Path;C:/Program Files/Datadog/Datadog Agent/bin;C:/Program Files/Datadog/Datadog Agent/bin/agent"

# Set variable indicating we are running in a container
setx /m DOCKER_DD_AGENT "true"
$Env:DOCKER_DD_AGENT="true"
