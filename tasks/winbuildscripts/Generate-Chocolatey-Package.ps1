agentVersion=(inv agent.version) | Select-String -Pattern "\d+.\d+.\d+" | ForEach-Object{$_.Matches[0].Value}
choco pack --version=$agentVersion --out=c:\mnt\build-out c:\mnt\chocolatey\datadog-agent.nuspec
