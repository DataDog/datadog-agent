$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$awscli = 'https://s3.amazonaws.com/aws-cli/AWSCLI64PY3.msi'

## installs awscli inside container
Write-Host -ForegroundColor Green Installing awscli
$out = 'awscli.msi'
(New-Object System.Net.WebClient).DownloadFile($awscli, $out)
Start-Process msiexec -ArgumentList '/q /i awscli.msi' -Wait
Remove-Item $out
setx PATH "$Env:Path;c:\program files\amazon\awscli\bin"
$Env:Path="$Env:Path;c:\program files\amazon\awscli\bin"
Write-Host -ForegroundColor Green Done Installing awscli
