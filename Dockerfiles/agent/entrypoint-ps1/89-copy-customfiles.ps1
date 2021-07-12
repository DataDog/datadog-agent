
# Copy the custom checks and confs in the datadog-agent folder
if (Test-Path C:\conf.d) { 
    Copy-Item -Recurse -Force C:\conf.d C:\ProgramData\Datadog
}
if (Test-Path C:\checks.d) { 
    Copy-Item -Recurse -Force C:\checks.d C:\ProgramData\Datadog
}
