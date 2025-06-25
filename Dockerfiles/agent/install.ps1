$ErrorActionPreference = "Stop"
Trap { Write-Host "Error in install.ps1: $_" }

function Install-Service {
  param(
    [Parameter(Mandatory=$true)][string] $SvcName,
    [Parameter(Mandatory=$true)][string] $BinPath,
    [Parameter(Mandatory=$false)][string[]] $Depends
  )
  if ( $Depends.count -gt 0 ){
      New-Service -Name $SvcName -StartupType Manual -BinaryPathName $BinPath -Depends $Depends
  } else {
      New-Service -Name $SvcName -StartupType Manual -BinaryPathName $BinPath
  }
  $eventSourceData = new-object System.Diagnostics.EventSourceCreationData("$SvcName", "Application")  
  $eventSourceData.CategoryResourceFile = $BinPath
  $eventSourceData.MessageResourceFile = $BinPath

  If (![System.Diagnostics.EventLog]::SourceExists($eventSourceData.Source))
  {      
  [System.Diagnostics.EventLog]::CreateEventSource($eventSourceData)  
  } 
}

if ("$env:WITH_JMX" -ne "false") {
    $JDK_UPSTREAM = "https://github.com/adoptium/temurin11-binaries/releases/download/jdk-11.0.25%2B9"
    $JDK_FILENAME = "OpenJDK11U-jre_x64_windows_hotspot_11.0.25_9.zip"
    $JDK_DIR = "jdk-11.0.25+9-jre"
    $JDK_SHA256 = "052f09448d5b8d9afb7a8e5049d40d7fafa8f5884afe6043bb2359787fd41e84"

    $JDK_DOWNLOAD_URL = if ($env:GENERAL_ARTIFACTS_CACHE_BUCKET_URL) {"${env:GENERAL_ARTIFACTS_CACHE_BUCKET_URL}/openjdk"} else {$JDK_UPSTREAM}
    Invoke-WebRequest -OutFile jre.zip "${JDK_DOWNLOAD_URL}/${JDK_FILENAME}"
    (Get-FileHash -Algorithm SHA256 jre.zip).Hash -eq "$JDK_SHA256"
    Expand-Archive -Path jre.zip -DestinationPath C:/
    Remove-Item jre.zip
    Move-Item "C:/$JDK_DIR/" C:/java
    # Add java to the path for jmxfetch
    setx /m PATH "$Env:Path;C:/java/bin"
    $Env:Path="$Env:Path;C:/java/bin"
}

New-Item -ItemType directory -Path 'C:/ProgramData/Datadog'
Move-Item "C:/Program Files/Datadog/Datadog Agent/etc/datadog-agent/conf.d" "C:/ProgramData/Datadog/conf.d"
# This folder only contains config artifacts, we've copied what we need so we can remove the rest.
rm -r -fo "C:/Program Files/Datadog/Datadog Agent/etc/"

$services = [ordered]@{
  "datadogagent" = "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe",@()
  "datadog-process-agent" = "C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe",@("datadogagent")
  "datadog-trace-agent" = "C:\Program Files\Datadog\Datadog Agent\bin\agent\trace-agent.exe",@("datadogagent")
}

foreach ($s in $services.Keys) {
    Install-Service -SvcName $s -BinPath $services[$s][0] $services[$s][1]
}

# Since OpenSSL 3.4, the install paths can be retrieved from the registry instead of being hardcoded at build time.
# https://github.com/openssl/openssl/blob/master/NOTES-WINDOWS.md#installation-directories
# TODO: How best to configure the OpenSSL version?
$opensslVersion = "3.4"
if ($env:WITH_FIPS -eq "true") {
  $opensslctx = "datadog-fips-agent"
} else {
  $opensslctx = "datadog-agent"
}
$keyPath = "HKLM:\SOFTWARE\Wow6432Node\OpenSSL-$opensslVersion-$opensslctx"

# Create the registry key
if (-not (Test-Path $keyPath)) {
  New-Item -Path $keyPath -Force
}

# Set the registry values
$embeddedPath = "C:\Program Files\Datadog\Datadog Agent\embedded3"
Set-ItemProperty -Path $keyPath -Name "OPENSSLDIR" -Value "$embeddedPath\ssl"
Set-ItemProperty -Path $keyPath -Name "ENGINESDIR" -Value "$embeddedPath\lib\engines-3"
Set-ItemProperty -Path $keyPath -Name "MODULESDIR" -Value "$embeddedPath\lib\ossl-modules"

# Allow to run agent binaries as `agent`
setx /m PATH "$Env:Path;C:/Program Files/Datadog/Datadog Agent/bin;C:/Program Files/Datadog/Datadog Agent/bin/agent"
$Env:Path="$Env:Path;C:/Program Files/Datadog/Datadog Agent/bin;C:/Program Files/Datadog/Datadog Agent/bin/agent"

# Set variable indicating we are running in a container
setx /m DOCKER_DD_AGENT "true"
$Env:DOCKER_DD_AGENT="true"

# Create install_info
Write-Output @"
---
install_method:
  tool: docker-win
  tool_version: docker-win-$env:INSTALL_INFO
  installer_version: docker-win-$env:INSTALL_INFO
"@ > C:/ProgramData/Datadog/install_info
