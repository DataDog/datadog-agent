#Download and Run MSI package for Automated install
new-module -name StsAgentInstaller -scriptblock {
    [Console]::OutputEncoding = New-Object -typename System.Text.ASCIIEncoding
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]'Tls,Tls11,Tls12'

    function Install-Project {
        param (
            [Parameter(Mandatory = $true)]
            [ValidateNotNullOrEmpty()]
            [string]$stsApiKey,

            [Parameter(Mandatory = $true)]
            [ValidateNotNullOrEmpty()]
            [string]$stsUrl,

            [string]$stsHostname = $env:computername,
            [string]$stsSkipSSLValidation = "false",
            [string]$stsCodeName = "stable",
            [string]$stsAgentVersion = "latest"
        )

        $stsDownloadBase = "$WIN_REPO"

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

            Else {                
                Write-Host "File $out doesn't exists - failed to download or corrupted. Please check."
                exit 1
                }
        }

        Download_MSI_STS_Installer
        Install_STS
    }

    set-alias install -value Install-Project

    export-modulemember -function 'Install-Project' -alias 'install'

}
