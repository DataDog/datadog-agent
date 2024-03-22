function Install-DDAgent
{
    param (
        [Parameter()][ValidateSet('DotNet')][String[]]$WithAPMTracers,
        [String] $AgentInstallURL,
        [String] $AgentInstallerPath,
        [Parameter(Mandatory)] [String]$ApiKey,
        [String]$Site,
        [String]$Tags,
        [String]$Hostname,
        [String]$DDAgentUsername,
        [String]$DDAgentPassword,
        [String]$ApplicationDataDirectory,
        [String]$ProjectLocation,
        [String]$InstallLogPath
    )

    $currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator))
    {
        $exception = [Exception]::new("Administrator priviledges required.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }

    # Enable TLS 1.2
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

    $installerPath = "datadog-agent.msi"
    if ($PSBoundParameters.ContainsKey('AgentInstallerPath'))
    {
        $installerPath = $AgentInstallerPath
    }
    else
    {
        Write-Host "Downloading Datadog Windows Agent installer"
        $url = "https://s3.amazonaws.com/ddagent-windows-stable/datadog-agent-7-latest.amd64.msi"
        if ($PSBoundParameters.ContainsKey('AgentInstallURL'))
        {
            $url = $AgentInstallURL
        }
        downloadAsset -url $url -outFile $installerPath
    }

    Write-Host "Installing Datadog Windows Agent"
    $installerParameters = formatAgentInstallerParameters -params $PSBoundParameters

    $logFile = ""
    if ($PSBoundParameters.ContainsKey('InstallLogPath'))
    {
        $logFile = $InstallLogPath
    }
    else
    {
        $logFile = createTemporaryLogFile -prefix "ddagent-msi"
    }

    $installResult = Start-Process -Wait msiexec -ArgumentList "/qn /i $installerPath $installerParameters /log $logFile" -PassThru
    if ($installResult.ExitCode -ne 0)
    {
        $exception = [Exception]::new("Agent installation failed. For more information, check the installation log file at $logFile.")
        $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
    }

    if ($PSBoundParameters.ContainsKey('WithAPMTracers') -and ($WithAPMTracers -contains "DotNet")) # note that -contains is a case insensitive operator
    {
        $latestVersionTag = ((Invoke-WebRequest https://api.github.com/repos/DataDog/dd-trace-dotnet/releases/latest).Content | ConvertFrom-Json).tag_name
        $latestVersion = $latestVersionTag.TrimStart("v")

        Write-Host "Downloading Datadog .NET Tracing Library $latestVersionTag"
        downloadAsset -url "https://github.com/DataDog/dd-trace-dotnet/releases/download/$latestVersionTag/datadog-dotnet-apm-$latestVersion-x64.msi" -outFile "datadog-dotnet-apm.msi"

        Write-Host "Installing .NET Tracing Library"
        $installResult = Start-Process -Wait msiexec -ArgumentList '/qn /i datadog-dotnet-apm.msi' -PassThru
        if ($installResult.ExitCode -ne 0)
        {
            $exception = [Exception]::new(".NET Tracing Library installation failed.")
            $PSCmdlet.ThrowTerminatingError([System.Management.Automation.ErrorRecord]::new($exception, "FatalError", [Management.Automation.ErrorCategory]::InvalidOperation, $null))
        }
    }

    <#
    .SYNOPSIS
    Installs the Datadog Agent.

    .DESCRIPTION
    Downloads the latest Datadog Windows Agent installer, validates the installer's signature, and executes the installation.

    .PARAMETER WithAPMTracers
    Specify an APM Tracing library to download and install alongside the Agent.

    .PARAMETER AgentInstallURL
    Override the URL which the Agent installer is downloaded from.

    .PARAMETER AgentInstallerPath
    Path to a local Datadog Windows Agent installer to run. If this option is provided, an Agent installer will not be downloaded.

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

    .PARAMETER AgentInstallURL
    Override the URL which the Agent installer is downloaded from.

    .PARAMETER InstallLogPath
    Override the Agent installation log location.

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

function formatAgentInstallerParameters($params)
{
    $formattedparams = "APIKEY=$($params.ApiKey)"

    if ($params.ContainsKey('Site'))                        { $formattedparams += " SITE=$($params.Site)"}
    if ($params.ContainsKey('Tags'))                        { $formattedparams += " TAGS=``$($params.Tags)``" }
    if ($params.ContainsKey('Hostname'))                    { $formattedparams += " HOSTNAME=$($params.Hostname)" }
    if ($params.ContainsKey('DDAgentUsername'))             { $formattedparams += " DDAGENTUSER_NAME=$($params.DDAgentUsername)" }
    if ($params.ContainsKey('DDAgentPassword'))             { $formattedparams += " DDAGENTUSER_PASSWORD=$($params.DDAgentPassword)" }
    if ($params.ContainsKey('ApplicationDataDirectory'))    { $formattedparams += " APPLICATIONDATADIRECTORY=$($params.ApplicationDataDirectory)" }
    if ($params.ContainsKey('ProjectLocation'))             { $formattedparams += " PROJECTLOCATION=$($params.ProjectLocation)" }

    return $formattedparams
}

function downloadAsset($url, $outFile)
{
    # Suppress progress bar to prevent performance issues (affects older windows versions, see https://github.com/PowerShell/PowerShell/issues/13414)
    $ProgressPreference = 'SilentlyContinue'

    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $outFile
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

    if ($signature.SignerCertificate.Subject -ne 'CN="Datadog, Inc", O="Datadog, Inc", L=New York, S=New York, C=US')
    {
        return $false
    }

    if ($signature.SignerCertificate.Issuer -ne 'CN=DigiCert Trusted G4 Code Signing RSA4096 SHA384 2021 CA1, O="DigiCert, Inc.", C=US')
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
