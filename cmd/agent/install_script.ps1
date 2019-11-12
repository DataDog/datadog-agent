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

            [string]$hostname = $env:computername,
            [string]$hostTags = "",
            [string]$codeName = "stable",
            [string]$skipSSLValidation = "false",
            [string]$agentVersion = "latest",
            [string]$f
        )

        $stsDownloadBase = "$env:WIN_REPO"
        If ( [string]::IsNullOrEmpty($stsDownloadBase) ) {
          $stsDownloadBase = "https://stackstate-agent-2.s3.amazonaws.com/windows"
        }
        $DirectFileExists = $False

        If (! [string]::IsNullOrEmpty($f)) {

        $DirectFileExists = Test-Path $f -IsValid
        If ($DirectFileExists -eq $False) {
          Write-Error "File $f doesn't exists - failed to download or corrupted. Please check." -ErrorAction Stop
          exit 1
        }

        $out = $f

        Write-Host "Script will take $out, and execute passing params $stsApiKey ; $stsUrl ; $hostname ; $hostTags ; $codeName ; $skipSSLValidation ; $agentVersion ;"

        } Else {

        Write-Host "Building download uri from $stsDownloadBase/$codeName/stackstate-agent-$agentVersion-1-x86_64.msi"
        $uri = "$stsDownloadBase/$codeName/stackstate-agent-$agentVersion-1-x86_64.msi"
        $out = "c:\stackstate-agent.msi"

        Write-Host "Script will download installer from $uri to $out, and execute passing params $stsApiKey ; $stsUrl ; $hostname ; $hostTags ; $codeName ; $skipSSLValidation ; $agentVersion ;"

        }

        $defaultHostTags = "os:windows"
        If ([string]::IsNullOrEmpty($hostTags)) {
            $hostTags = $defaultHostTags
        } Else {
            $hostTags = "$defaultHostTags,$hostTags"
        }



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
                " APIKEY=`"$stsApiKey`" STS_URL=`"$stsUrl`" HOSTNAME=`"$hostname`" TAGS=`"$hostTags`" SKIP_SSL_VALIDATION=`"$skipSSLValidation`" "
            )
            write-host "About to install $msifile with arguments "$MSIArguments
            If ($FileExists -eq $True) {
                Start-Process "msiexec.exe" -ArgumentList $MSIArguments -passthru | wait-process
                Write-Host "Finished msi "$msifile
            } Else {
                Write-Error "File $out doesn't exists - failed to download or corrupted. Please check." -ErrorAction Stop
                exit 1
            }
        }

        Set-StrictMode -Version Latest
        $ErrorActionPreference = "Stop"
        $PSDefaultParameterValues['*:ErrorAction'] = 'Stop'

        If ($DirectFileExists -eq $False) {
        Download_MSI_STS_Installer
        }
        Install_STS
    }

    set-alias install -value Install-Project

    export-modulemember -function 'Install-Project' -alias 'install'
}