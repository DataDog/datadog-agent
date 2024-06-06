function Install-DDAgent
{
    param (
        # Parameters that are always valid
        [String] $AgentInstallerURL,
        [String] $AgentInstallerPath,
        [String] $AgentVersion,
        [String] $AgentInstallLogPath,
        [Parameter()][ValidateSet('DotNet')][String[]]$WithAPMTracers,
        [switch] $RestartIIS,

        # Parameters that are only valid on first install
        [String]$ApiKey,
        [String]$Site,
        [String]$Tags,
        [String]$Hostname,
        [String]$DDAgentUsername,
        [String]$DDAgentPassword,
        [String]$ApplicationDataDirectory,
        [String]$ProjectLocation
    )

    validateAdminPriviledges
    validate64BitProcess
    enableTSL12

    [string] $uniqueID = [System.Guid]::NewGuid()

    installAgent -params $PSBoundParameters -uniqueID $uniqueID

    if ($PSBoundParameters.ContainsKey('WithAPMTracers'))
    {
        if ($WithAPMTracers -contains "DotNet") # note that -contains is a case insensitive operator
        {
            installDotnetTracer -uniqueID $uniqueID
            if ($RestartIIS)
            {
                Write-Host "Restarting IIS"
                stop-service -force was
                Restart-Service -Name W3SVC -Force
            }
        }
    }

    <#
    .SYNOPSIS
    Installs the Datadog Agent.

    .DESCRIPTION
    Downloads the latest Datadog Windows Agent installer, validates the installer's signature, and executes the installation.

    .PARAMETER AgentInstallerURL
    Override the URL which the Agent installer is downloaded from.

    .PARAMETER AgentInstallerPath
    Path to a local Datadog Windows Agent installer to run. If this option is provided, an Agent installer will not be downloaded.

    .PARAMETER AgentVersion
    The Agent version to download. Example: 7.52.1

    .PARAMETER AgentInstallLogPath
    Override the Agent installation log location.

    .PARAMETER WithAPMTracers
    Specify an APM Tracing library to download and install alongside the Agent.

    .PARAMETER ApiKey
    Adds the Datadog API KEY to the configuration file.

    .PARAMETER Site
    Set the Datadog intake site. Example: SITE=datadoghq.com

    .PARAMETER Tags
    Comma-separated list of tags to assign in the configuration file. Example: TAGS="key_1:val_1,key_2:val_2"

    .PARAMETER Hostname
    Configures the hostname reported by the Agent to Datadog (overrides any hostname calculated at runtime).

    .PARAMETER DDAgentUsername
    Override the default ddagentuser username used during Agent installation (v6.11.0+).

    .PARAMETER DDAgentPassword
    Override the cryptographically secure password generated for the ddagentuser user during Agent installation (v6.11.0+). Must be provided for installs on domain servers/domain controllers.

    .PARAMETER ApplicationDataDirectory
    Override the directory to use for the configuration file directory tree (v6.11.0+). May only be provided on initial install; not valid for upgrades. Default: C:\ProgramData\Datadog

    .PARAMETER ProjectLocation
    Override the directory to use for the binary file directory tree (v6.11.0+). May only be provided on initial install; not valid for upgrades. Default: %ProgramFiles%\Datadog\Datadog Agent

    .INPUTS
    None. You cannot pipe objects to Install-DDAgent.

    .OUTPUTS
    None.

    .EXAMPLE
    C:\PS> Install-DDAgent -ApiKey "<YOUR_DATADOG_API_KEY>"

    .EXAMPLE
    C:\PS> Install-DDAgent -ApiKey "<YOUR_DATADOG_API_KEY>" -WithAPMTracers "DotNet"

    .LINK
    Learn more about the Datadog Windows Agent User:
    https://docs.datadoghq.com/agent/guide/windows-agent-ddagent-user/
    #>
}

###############################
# Unexported helper functions #
###############################

function validateAdminPriviledges()
{
    $currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator))
    {
        $exception = [Exception]::new("Administrator priviledges required.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }
}

function validate64BitProcess()
{
    if (-not [Environment]::Is64BitProcess)
    {
        $exception = [Exception]::new("This command must be run in a 64-bit environment.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }
}

function enableTSL12()
{
    # Powershell does not enabled TLS 1.2 by default, & we want it enabled for faster downloads
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12
}

function installAgent($params, $uniqueID)
{
    $installerPath = Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath "datadog-agent-$uniqueID.msi"
    $downloadInstaller = $true
    if ($params.ContainsKey('AgentInstallerPath'))
    {
        $installerPath = $params.AgentInstallerPath
        $downloadInstaller = $false
    }

    if ($downloadInstaller -eq $true)
    {
        $version = "datadog-agent-7-latest.amd64"
        if ($params.ContainsKey('AgentVersion'))
        {
            $version = "ddagent-cli-$($params.AgentVersion)"
        }
        $url = "https://s3.amazonaws.com/ddagent-windows-stable/$version.msi"
        if ($params.ContainsKey('AgentInstallerURL'))
        {
            $url = $params.AgentInstallerURL
        }

        Write-Host "Downloading Datadog Windows Agent installer"
        downloadAsset -url $url -outFile $installerPath
    }

    $logFile = ""
    if ($params.ContainsKey('AgentInstallLogPath'))
    {
        $logFile = $params.AgentInstallLogPath
    }
    else
    {
        $logFile = createTemporaryLogFile -prefix "ddagent-msi"
    }

    $defaultInstallArgs = @("/qn", "/log `"$logFile`"", "/i `"$installerPath`"")
    $customInstallArgs = formatAgentInstallerArguments -params $params

    Write-Host "Installing Datadog Windows Agent"
    $installResult = Start-Process -Wait msiexec -ArgumentList $($defaultInstallArgs; $customInstallArgs) -PassThru

    if ($downloadInstaller)
    {
        Remove-Item $installerPath
    }

    if ($installResult.ExitCode -ne 0)
    {
        $exception = [Exception]::new("Agent installation failed. For more information, check the installation log file at $logFile.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }
    ## write the install info file
    $installInfo = @"
---
install_method:
  tool: powershell-Datadog
  tool_version: $($MyInvocation.MyCommand.Module.Version)
  installer_version: Datadog_powershell_module
"@

    $appDataDir = (Get-ItemProperty -Path "HKLM:\SOFTWARE\Datadog\Datadog Agent").ConfigRoot
    Out-File -FilePath $appDataDir\install_info -InputObject $installInfo
}

function installDotnetTracer($uniqueID)
{
    $latestVersionTag = ((Invoke-WebRequest -UseBasicParsing https://api.github.com/repos/DataDog/dd-trace-dotnet/releases/latest).Content | ConvertFrom-Json).tag_name
    $latestVersion = $latestVersionTag.TrimStart("v")

    $installerPath = Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath "datadog-dotnet-apm-$uniqueID.msi"

    Write-Host "Downloading Datadog .NET Tracing Library installer ($latestVersionTag)"
    downloadAsset -url "https://github.com/DataDog/dd-trace-dotnet/releases/download/$latestVersionTag/datadog-dotnet-apm-$latestVersion-x64.msi" -outFile "$installerPath"

    Write-Host "Installing Datadog .NET Tracing Library"
    $installResult = Start-Process -Wait msiexec -ArgumentList "/qn /i $installerPath" -PassThru

    Remove-Item $installerPath

    if ($installResult.ExitCode -ne 0)
    {
        $exception = [Exception]::new(".NET Tracing Library installation failed.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }
}

function doesDatadogYamlExist()
{
    try {
        $configRoot = (Get-ItemProperty -ErrorAction Stop -Path 'HKLM:\SOFTWARE\Datadog\Datadog Agent').ConfigRoot
        if ($configRoot -ne "")
        {
            return Test-Path -Path (Join-Path -Path $configRoot -ChildPath "datadog.yaml")
        }
        return $false
    }
    catch {
        return $false
    }
}

function formatAgentInstallerArguments($params)
{
    [string[]] $formattedArgs = @()

    if ($params.ContainsKey('ApiKey'))                      { $formattedArgs += "APIKEY=$($params.ApiKey)"}
    if ($params.ContainsKey('Site'))                        { $formattedArgs += "SITE=$($params.Site)"}
    if ($params.ContainsKey('Hostname'))                    { $formattedArgs += "HOSTNAME=$($params.Hostname)" }
    if ($params.ContainsKey('DDAgentUsername'))             { $formattedArgs += "DDAGENTUSER_NAME=$($params.DDAgentUsername)" }
    if ($params.ContainsKey('DDAgentPassword'))             { $formattedArgs += "DDAGENTUSER_PASSWORD=$($params.DDAgentPassword)" }
    if ($params.ContainsKey('Tags'))                        { $formattedArgs += "TAGS=`"$($params.Tags)`"" }
    if ($params.ContainsKey('ApplicationDataDirectory'))    { $formattedArgs += "APPLICATIONDATADIRECTORY=`"$($params.ApplicationDataDirectory)`"" }
    if ($params.ContainsKey('ProjectLocation'))             { $formattedArgs += "PROJECTLOCATION=`"$($params.ProjectLocation)`"" }
    
    if (($formattedArgs.Count -ne 0) -and (doesDatadogYamlExist -eq $true))
    {
        Write-Warning "A datadog.yaml file already exists. The contents of that file will take precedence over the following parameters: $formattedArgs"
        # We will still pass the parameters along to the installer, and let it decide what to do with them
    }

    return $formattedArgs
}

function downloadAsset($url, $outFile)
{
    (New-Object System.Net.WebClient).DownloadFile($url, $outFile)
    if (-not $?)
    {
        $exception = [Exception]::new("Download failed.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }

    if (-not (hasValidDDSignature -asset $outFile))
    {
        $exception = [Exception]::new("$outFile did not pass the signature check.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }
}

function hasValidDDSignature($asset)
{
    if (-not (Test-Path $asset -PathType Leaf)) {
        $exception = [Exception]::new("$asset not found")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }

    $signature = Get-AuthenticodeSignature $asset

    if ($signature.Status -ne "Valid")
    {
        return $false
    }

    if ($signature.SignerCertificate.Subject -contains 'Datadog') # case insensitive check
    {
        return $false
    }

    return $true
}

function createTemporaryLogFile($prefix)
{
    $tempFile = New-TemporaryFile
    $renamedTempFile = Rename-Item -Path $tempFile -NewName "$prefix-$((Get-ChildItem $tempFile).BaseName).log" -PassThru
    return $renamedTempFile.FullName
}
