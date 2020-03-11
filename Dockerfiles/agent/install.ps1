$ErrorActionPreference = "Stop"
Trap { Write-Host "Error in install.ps1: $_" }

if ("$env:WITH_JMX" -ne "false") {
    Invoke-WebRequest -OutFile jre-11.0.6.zip https://github.com/AdoptOpenJDK/openjdk11-binaries/releases/download/jdk-11.0.6%2B10/OpenJDK11U-jre_x64_windows_hotspot_11.0.6_10.zip
    Expand-Archive -Path jre-11.0.6.zip -DestinationPath C:/
    Remove-Item jre-11.0.6.zip
    Move-Item C:/jdk-11.0.6+10-jre/ C:/java
    setx PATH "$env:PATH;C:\java\bin"
    java -version
}

Expand-Archive datadog-agent-7-latest.amd64.zip
Remove-Item datadog-agent-7-latest.amd64.zip

Get-ChildItem -Path datadog-agent-7-* | Rename-Item -NewName "Datadog Agent"

New-Item -ItemType directory -Path "C:/Program Files/Datadog"
Move-Item "Datadog Agent" "C:/Program Files/Datadog/"

New-Item -ItemType directory -Path 'C:/ProgramData/Datadog' 
Move-Item "C:/Program Files/Datadog/Datadog Agent/EXAMPLECONFSLOCATION" "C:/ProgramData/Datadog/conf.d"
