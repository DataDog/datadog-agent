$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12


$wdk ='https://go.microsoft.com/fwlink/?linkid=2026156'

Write-Host -ForegroundColor Green Installing WDK
$out = 'wdksetup.exe'
Write-Host -ForegroundColor Green Downloading $wdk to $out
(New-Object System.Net.WebClient).DownloadFile($wdk, $out)
Get-ChildItem $out
# write file size to make sure it worked
Write-Host -ForegroundColor Green "File size is $((get-item $out).length)"



Start-Process wdksetup.exe -ArgumentList '/q' -Wait

#install WDK.vsix (hack)
mkdir c:\tmp
copy "C:\Program Files (x86)\Windows Kits\10\Vsix\WDK.vsix" C:\TMP\wdkvsix.zip
Expand-Archive C:\TMP\wdkvsix.zip -DestinationPath C:\TMP\wdkvsix
robocopy /e "C:\TMP\wdkvsix\`$VCTargets\Platforms" "${Env:VSTUDIO_ROOT}\Common7\IDE\VC\VCTargets\Platforms" 
remove-item -Force -Recurse -Path "c:\tmp"

#.`clean_tmps.ps1
Write-Host -ForegroundColor Green Done with WDK