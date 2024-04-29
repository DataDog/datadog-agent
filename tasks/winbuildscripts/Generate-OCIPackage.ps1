Write-HOst "Downloading datadog-package.exe"
(New-Object System.Net.WebClient).DownloadFile("https://dd-agent-omnibus.s3.amazonaws.com/datadog-package.exe", "C:\\tools\\datadog-package.exe")
$rawAgentVersion = "{0}-1" -f (inv agent.version --url-safe --major-version 7)
Write-Host "Detected agent version ${rawAgentVersion}"
& C:\\tools\\datadog-package.exe create --package datadog-agent --version $rawAgentVersion c:\buildroot\datadog-agent\omnibus\pkg\
