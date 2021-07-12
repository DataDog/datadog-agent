<#
.SYNOPSIS
    Invokes the specified batch file and retains any environment variable changes it makes.
.DESCRIPTION
    Invoke the specified batch file (and parameters), but also propagate any
    environment variable changes back to the PowerShell environment that
    called it.
.PARAMETER Path
    Path to a .bat or .cmd file.
.PARAMETER Parameters
    Parameters to pass to the batch file.
.EXAMPLE
    C:\PS> Invoke-BatchFile "$env:ProgramFiles\Microsoft Visual Studio 9.0\VC\vcvarsall.bat"
    Invokes the vcvarsall.bat file.  All environment variable changes it makes will be
    propagated to the current PowerShell session.
.NOTES
    Author: Lee Holmes
#>
function Invoke-BatchFile
{
    param([string]$Path, [string]$Parameters)

    $tempFile = [IO.Path]::GetTempFileName()

    ## Store the output of cmd.exe.  We also ask cmd.exe to output
    ## the environment table after the batch file completes
    cmd.exe /c " `"$Path`" $Parameters && set " > $tempFile

    ## Go through the environment variables in the temp file.
    ## For each of them, set the variable in our local environment.
    Get-Content $tempFile | Foreach-Object {
        if ($_ -match "^(.*?)=(.*)$") {
            Set-Content "env:\$($matches[1])" $matches[2]
        }
        else {
            $_
        }
    }

    Remove-Item $tempFile
}



<#
.SYNOPSIS
    Imports environment variables for the specified version of Visual Studio.
.DESCRIPTION
    Imports environment variables for the specified version of Visual Studio.
    This function requires the PowerShell Community Extensions. To find out
    the most recent set of Visual Studio environment variables imported use
    the cmdlet Get-EnvironmentBlock.  If you want to revert back to a previous
    Visul Studio environment variable configuration use the cmdlet
    Pop-EnvironmentBlock.
.PARAMETER VisualStudioVersion
    The version of Visual Studio to import environment variables for. Valid
    values are 2008, 2010, 2012, 2013, 2015, 2017 and 2019.
.PARAMETER Architecture
    Selects the desired architecture to configure the environment for.
    If this parameter isn't specified, the command will attempt to locate and
    use VsDevCmd.bat.  If VsDevCmd.bat can't be found (not installed) then the
    command will use vcvarsall.bat with either the argument x86 if running in
    32-bit PowerShell or amd64 if running in 64-bit PowerShell. Other valid
    values are: arm, x86_arm, x86_amd64, amd64_x86.
.PARAMETER RequireWorkload
    This parameter applies to Visual Studio 2017 and higher.  It allows you
    to specify which workloads are required for the environment you desire to
    import.  This can be used when you have multiple versions of Visual Studio
    2017 installed and different versions support different workloads e.g.
    perhaps only the "Preview" version supports the
    Microsoft.VisualStudio.Component.VC.Tools.x86.x64 workload.
.EXAMPLE
    C:\PS> Import-VisualStudioVars 2015

    Sets up the environment variables to use the VS 2015 tools. If
    VsDevCmd.bat is found then it will use that. Otherwise, vcvarsall.bat will
    be used with an architecture of either x86 for 32-bit Powershell, or amd64
    for 64-bit Powershell.
.EXAMPLE
    C:\PS> Import-VisualStudioVars 2013 arm

    Sets up the environment variables for the VS 2013 ARM tools.
.EXAMPLE
    C:\PS> Import-VisualStudioVars 2017 -Architecture amd64 -RequireWorkload Microsoft.VisualStudio.Component.VC.Tools.x86.x64

    Finds an instance of VS 2017 that has the required workload and sets up
    the environment variables to use that instance of the VS 2017 tools.
    To see a full list of available workloads, execute:
    Get-VSSetupInstance | Foreach-Object Packages | Foreach-Object Id | Sort-Object
#>
function Import-VisualStudioVars
{
    param
    (
        [Parameter(Position = 0)]
        [ValidateSet('90', '2008', '100', '2010', '110', '2012', '120', '2013', '140', '2015', '150', '2017','160','2019')]
        [string]
        $VisualStudioVersion,

        [Parameter(Position = 1)]
        [string]
        $Architecture,

        [Parameter()]
        [ValidateNotNullOrEmpty()]
        [string[]]
        $RequireWorkload
    )

    begin {
        $ArchSpecified = $true
        if (!$Architecture) {
            $ArchSpecified = $false
            $Architecture = $(if ($Pscx:Is64BitProcess) {'amd64'} else {'x86'})
        }

        function GetSpecifiedVSSetupInstance($Version, [switch]$Latest, [switch]$FailOnMissingVSSetup) {
            if ((Get-Module -Name VSSetup -ListAvailable) -eq $null) {
                Write-Warning "You must install the VSSetup module to import Visual Studio variables for Visual Studio 2017 or higher."
                Write-Warning "Install the VSSetup module with the command: Install-Module VSSetup -Scope CurrentUser"

                if ($FailOnMissingVSSetup) {
                    throw "VSSetup module is not installed, unable to import Visual Studio 2017 (or higher) environment variables."
                }
                else {
                    # For the default (no VS version specified) case, we can look for earlier versions of VS.
                    return $null
                }
            }

            Import-Module VSSetup -ErrorAction Stop

            $selectArgs = @{
                Product = '*'
            }

            if ($Latest) {
                $selectArgs['Latest'] = $true
            }
            elseif ($Version) {
                $selectArgs['Version'] = $Version
            }

            if ($RequireWorkload -or $ArchSpecified) {
                if (!$RequireWorkload) {
                    # We get here when the architecture was specified but no worload, most likely these users want the C++ workload
                    $RequireWorkload = 'Microsoft.VisualStudio.Component.VC.Tools.x86.x64'
                }

                $selectArgs['Require'] = $RequireWorkload
            }

            Write-Verbose "$($MyInvocation.MyCommand.Name) Select-VSSetupInstance args:"
            Write-Verbose "$(($selectArgs | Out-String) -split "`n")"
            $vsInstance = Get-VSSetupInstance | Select-VSSetupInstance @selectArgs | Select-Object -First 1
            $vsInstance
        }

        function FindAndLoadBatchFile($ComnTools, $ArchSpecified, [switch]$IsAppxInstall) {
            $batchFilePath = Join-Path $ComnTools VsDevCmd.bat
            if (!$ArchSpecified -and (Test-Path -LiteralPath $batchFilePath)) {
                if ($IsAppxInstall) {
                    # The newer batch files spit out a header that tells you which environment was loaded
                    # so only write out the below message when -Verbose is specified.
                    Write-Verbose "Invoking '$batchFilePath'"
                }
                else {
                    "Invoking '$batchFilePath'"
                }

                Invoke-BatchFile $batchFilePath
            }
            else {
                if ($IsAppxInstall) {
                    $batchFilePath = Join-Path $ComnTools ..\..\VC\Auxiliary\Build\vcvarsall.bat

                    # The newer batch files spit out a header that tells you which environment was loaded
                    # so only write out the below message when -Verbose is specified.
                    Write-Verbose "Invoking '$batchFilePath' $Architecture"
                }
                else {
                    $batchFilePath = Join-Path $ComnTools ..\..\VC\vcvarsall.bat
                    "Invoking '$batchFilePath' $Architecture"
                }

                Invoke-BatchFile $batchFilePath $Architecture
            }
        }
    }

    end
    {
        switch -regex ($VisualStudioVersion)
        {
            '90|2008' {
                Push-EnvironmentBlock -Description "Before importing VS 2008 $Architecture environment variables"
                Write-Verbose "Invoking ${env:VS90COMNTOOLS}..\..\VC\vcvarsall.bat $Architecture"
                Invoke-BatchFile "${env:VS90COMNTOOLS}..\..\VC\vcvarsall.bat" $Architecture
            }

            '100|2010' {
                Push-EnvironmentBlock -Description "Before importing VS 2010 $Architecture environment variables"
                Write-Verbose "Invoking ${env:VS100COMNTOOLS}..\..\VC\vcvarsall.bat $Architecture"
                Invoke-BatchFile "${env:VS100COMNTOOLS}..\..\VC\vcvarsall.bat" $Architecture
            }

            '110|2012' {
                Push-EnvironmentBlock -Description "Before importing VS 2012 $Architecture environment variables"
                FindAndLoadBatchFile $env:VS110COMNTOOLS $ArchSpecified
            }

            '120|2013' {
                Push-EnvironmentBlock -Description "Before importing VS 2013 $Architecture environment variables"
                FindAndLoadBatchFile $env:VS120COMNTOOLS $ArchSpecified
            }

            '140|2015' {
                Push-EnvironmentBlock -Description "Before importing VS 2015 $Architecture environment variables"
                FindAndLoadBatchFile $env:VS140COMNTOOLS $ArchSpecified
            }

            '150|2017' {
                $vsInstance = GetSpecifiedVSSetupInstance -Version '[15.0,16.0)' -FailOnMissingVSSetup
                if (!$vsInstance) {
                    throw "No instances of Visual Studio 2017 found$(if ($RequireWorkload) {" for the required workload: $RequireWorkload"})."
                }

                Push-EnvironmentBlock -Description "Before importing VS 2017 $Architecture environment variables"
                $installPath = $vsInstance.InstallationPath
                FindAndLoadBatchFile "$installPath/Common7/Tools" $ArchSpecified -IsAppxInstall
            }

            '160|2019' {
                $vsInstance = GetSpecifiedVSSetupInstance -Version '[16.0,17.0)' -FailOnMissingVSSetup
                if (!$vsInstance) {
                    throw "No instances of Visual Studio 2019 found$(if ($RequireWorkload) {" for the required workload: $RequireWorkload"})."
                }

                Push-EnvironmentBlock -Description "Before importing VS 2019 $Architecture environment variables"
                $installPath = $vsInstance.InstallationPath
                FindAndLoadBatchFile "$installPath/Common7/Tools" $ArchSpecified -IsAppxInstall
            }

            default {
                $vsInstance = GetSpecifiedVSSetupInstance -Latest
                if ($vsInstance) {
                    Push-EnvironmentBlock -Description "Before importing $($vsInstance.DisplayName) $Architecture environment variables"
                    $installPath = $vsInstance.InstallationPath
                    FindAndLoadBatchFile "$installPath/Common7/Tools" $ArchSpecified -IsAppxInstall
                }
                else {
                    $envvar = @(Get-Item Env:\vs*comntools | Sort-Object { $_.Name -replace '(?<=VS)(\d)(0)','0$1$2'} -Descending)[0]
                    if (!$envvar) {
                        throw "No versions of Visual Studio found."
                    }

                    Push-EnvironmentBlock -Description "Before importing $($envvar.Name) $Architecture environment variables"
                    FindAndLoadBatchFile ($envvar.Value) $ArchSpecified
                }
            }
        }
    }
}
