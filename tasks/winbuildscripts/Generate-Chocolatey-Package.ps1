$outputDirectory = "c:\mnt\build-out"
$agentVersion=(inv agent.version) | Select-String -Pattern "\d+.\d+.\d+" | ForEach-Object{$_.Matches[0].Value}
Write-Host "Generating Chocolatey package for $agentVersion in $outputDirectory"

if (!(Test-Path $outputDirectory)){
    New-Item -ItemType Directory -Path $outputDirectory
}

choco pack --version=$agentVersion --out=$outputDirectory c:\mnt\chocolatey\datadog-agent.nuspec
