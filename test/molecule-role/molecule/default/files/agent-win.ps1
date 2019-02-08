#Download and Run MSI package for Automated install
param (
    [Parameter(Mandatory = $true)]
    [string]$stsApiKey = "API_KEY",
    [string]$stsUrl = "https://test-stackstate-agent.sts/stsAgent",
    [string]$stsHostname = $env:computername,
    [string]$stsSkipSSLValidation = "true",
    [string]$stsCodeName = "master",
    [string]$stsAgentVersion = "2.0.0.git.443.ef0c11ef"
)

if ($stsCodeName = "master") {
    $stsDownloadBase = "https://s3-eu-west-1.amazonaws.com/stackstate-agent-2-test/windows" #TODO: point to release base
}
else {
    $stsDownloadBase = "https://s3-eu-west-1.amazonaws.com/stackstate-agent-2-test/windows"
}

Write-Host "Building download uri from $stsDownloadBase/$stsCodeName/stackstate-agent-$stsAgentVersion-1-x86_64.msi"
$uri = "$stsDownloadBase/$stsCodeName/stackstate-agent-$stsAgentVersion-1-x86_64.msi"
$out = "c:\stackstate-agent.msi"

Write-Host "Script will download installer from $uri to $out, and execute passing params $stsApiKey ; $stsUrl ; $stsHostname ; $stsSkipSSLValidation ; $stsCodeName ; $stsAgentVersion ;"

Function Download_MSI_STS_Installer {
    Write-Host "About to download $uri to $out"
    Invoke-WebRequest -uri $uri -OutFile $out
    $msifile = Get-ChildItem -Path $out -File -Filter '*.ms*'
    Write-Host "StackState MSI $msifile "
}

Function Install_STS {
    $msifile = Get-ChildItem -Path $out -File -Filter '*.ms*'
    $FileExists = Test-Path $msifile -IsValid

    $DataStamp = get-date -Format yyyyMMddTHHmmss
    $logFile = '{0}-{1}.log' -f $msifile.fullname, $DataStamp
    $MSIArguments = @(
        "/i"
        ('"{0}"' -f $msifile)
        "/qn"
        "/norestart"
        "/L*v"
        $logFile
        " STS_API_KEY=$stsApiKey STS_URL=$stsUrl STS_HOSTNAME=$stsHostname SKIP_SSL_VALIDATION=$stsSkipSSLValidation "
    )
    write-host "About to install $msifile with arguments "$MSIArguments
    If ($FileExists -eq $True) {
        Start-Process "msiexec.exe" -ArgumentList $MSIArguments -passthru | wait-process
        Write-Host "Finished msi "$msifile
    }

    Else {Write-Host "File $out doesn't exists - failed to download or corrupted. Please check."}
}

Download_MSI_STS_Installer
Install_STS
