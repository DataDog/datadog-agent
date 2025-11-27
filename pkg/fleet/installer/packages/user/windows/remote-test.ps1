<#
.SYNOPSIS
    Runs Go tests on a remote Windows host via SSH

.DESCRIPTION
    This script builds Go test executables and runs them on a remote Windows host via SSH.
    It supports both regular unit tests and manual tests that require a real Windows environment.
    The script assumes PowerShell is the default shell on the remote Windows host.
    Environment variables can be set on the remote host before running the tests.
    Complex commands (like test execution) use temporary PowerShell scripts to avoid quote escaping issues.

.PARAMETER RemoteHost
    The remote host IP address or hostname

.PARAMETER RemoteUser
    The username for SSH authentication

.PARAMETER RemotePort
    The SSH port (default: 22)

.PARAMETER TestPackage
    The Go package to test (default: current directory)

.PARAMETER TestTimeout
    Test timeout duration (default: 30m)

.PARAMETER BuildTags
    Build tags to use when building tests (e.g., "manualtest")

.PARAMETER TestArgs
    Additional arguments to pass to the test executable

.PARAMETER RemoteWorkDir
    Remote working directory for test execution (default: C:\temp\go-tests)

.PARAMETER KeepRemoteFiles
    Keep test files on remote host after execution

.PARAMETER EnvVars
    Hashtable of environment variables to set before running tests (e.g., @{VAR1="value1"; VAR2="value2"})

.PARAMETER Verbose
    Enable verbose output

.EXAMPLE
    .\remote-test.ps1 -RemoteHost 192.168.1.100 -RemoteUser testuser

.EXAMPLE
    .\remote-test.ps1 -RemoteHost testvm.local -RemoteUser admin -BuildTags "manualtest" -TestArgs "-v -test.run TestValidate"

.EXAMPLE
    .\remote-test.ps1 -RemoteHost 10.0.0.50 -RemoteUser testuser -TestTimeout "60m" -Verbose

.EXAMPLE
    .\remote-test.ps1 -RemoteHost testvm.local -RemoteUser admin -EnvVars @{DD_AGENT_USER_NAME='$env:COMPUTERNAME\Administrator'; CI="true"}
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)]
    [string]$RemoteHost,

    [Parameter(Mandatory=$true)]
    [string]$RemoteUser,

    [Parameter(Mandatory=$false)]
    [int]$RemotePort = 22,

    [Parameter(Mandatory=$false)]
    [string]$TestPackage = ".",

    [Parameter(Mandatory=$false)]
    [string]$TestTimeout = "30m",

    [Parameter(Mandatory=$false)]
    [string]$BuildTags = "",

    [Parameter(Mandatory=$false)]
    [string]$TestArgs = "",

    [Parameter(Mandatory=$false)]
    [string]$RemoteWorkDir = "C:\temp\go-tests",

    [Parameter(Mandatory=$false)]
    [switch]$KeepRemoteFiles,

    [Parameter(Mandatory=$false)]
    [hashtable]$EnvVars = @{}
)

# Set error action preference
$ErrorActionPreference = "Stop"

# Enable verbose output if requested
if ($Verbose) {
    $VerbosePreference = "Continue"
}

# Function to write colored output
function Write-ColorOutput {
    param(
        [string]$Message,
        [string]$Color = "White"
    )
    Write-Host $Message -ForegroundColor $Color
}

# Function to execute simple SSH commands directly
function Invoke-SSHCommand {
    param(
        [string]$Command,
        [string]$Description = ""
    )

    if ($Description) {
        Write-ColorOutput "[$Description]" "Cyan"
    }

    $sshArgs = @(
        "-o", "StrictHostKeyChecking=no",
        "-o", "UserKnownHostsFile=NUL",
        "-o", "LogLevel=quiet",
        "-p", $RemotePort,
        "$RemoteUser@$RemoteHost",
        $Command
    )

    Write-Verbose "Executing SSH command: ssh $($sshArgs -join ' ')"

    $p = Start-Process -Wait -PassThru -NoNewWindow ssh ($sshArgs -join ' ')
    if ($p.ExitCode -ne 0) {
        throw "SSH command failed with exit code $($p.ExitCode)"
    }
}

# Function to execute complex SSH commands by creating a local temp PowerShell file and SCPing it
# This avoids quote escaping issues for complex commands.
function Invoke-SSHScriptCommand {
    param(
        [string]$Command,
        [string]$Description = ""
    )

    if ($Description) {
        Write-ColorOutput "[$Description]" "Cyan"
    }

    # Generate unique temp file names
    $tempFileName = "temp_$(Get-Date -Format 'yyyyMMdd_HHmmss')_$([System.Guid]::NewGuid().ToString('N').Substring(0,8)).ps1"
    $localTempFile = Join-Path $env:TEMP $tempFileName
    $remoteTempFile = "$RemoteWorkDir\$tempFileName"

    Write-Verbose "Creating local temporary PowerShell file: $localTempFile"
    Write-Verbose "Remote file path: $remoteTempFile"
    Write-Verbose "Command content: $Command"

    try {
        # Create the PowerShell script content locally
        $scriptContent = @"
# Auto-generated PowerShell script for remote execution
# Generated at $(Get-Date)

$Command
if (-not `$?) { exit `$LASTEXITCODE }
"@

        # Write the script to local temp file
        Set-Content -Path $localTempFile -Value $scriptContent -Encoding UTF-16

        # First, ensure the remote directory exists (using simple SSH command)
        Invoke-SSHCommand "if (-not (Test-Path '$RemoteWorkDir')) { New-Item -ItemType Directory -Path '$RemoteWorkDir' -Force | Out-Null }"

        try {
            # Copy the script file to remote host using SCP
            Write-Verbose "Copying script file to remote host"
            Copy-FileViaSCP $localTempFile $remoteTempFile
            Invoke-SSHCommand "& '$remoteTempFile'" "Running tests"
        } finally {
            # Clean up the remote temporary file
            if (-Not $KeepRemoteFiles) {
                Invoke-SSHCommand "Remove-Item '$remoteTempFile' -Force -ErrorAction SilentlyContinue"
            }
        }
    } finally {
        # Clean up local temporary file
        if (Test-Path $localTempFile) {
            Remove-Item $localTempFile -Force -ErrorAction SilentlyContinue
            Write-Verbose "Cleaned up local temporary file: $localTempFile"
        }
    }
}

# Function to copy file via SCP
function Copy-FileViaSCP {
    param(
        [string]$LocalFile,
        [string]$RemoteFile,
        [string]$Description = ""
    )

    if ($Description) {
        Write-ColorOutput "[$Description]" "Cyan"
    }

    $scpArgs = @(
        "-o", "StrictHostKeyChecking=no",
        "-o", "UserKnownHostsFile=NUL",
        "-P", $RemotePort,
        $LocalFile,
        "$RemoteUser@$RemoteHost`:$RemoteFile"
    )

    Write-Verbose "Executing SCP command: scp $($scpArgs -join ' ')"

    & scp @scpArgs
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw "SCP command failed with exit code $exitCode"
    }
}

# Main execution
try {
    Write-ColorOutput "Starting remote test execution..." "Green"

    # Check if SSH is available
    if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
        throw "SSH command not found. Please ensure OpenSSH client is installed."
    }

    if (-not (Get-Command scp -ErrorAction SilentlyContinue)) {
        throw "SCP command not found. Please ensure OpenSSH client is installed."
    }

    # Get current directory and test package information
    $currentDir = Get-Location
    $testPackagePath = Resolve-Path $TestPackage
    Write-Verbose "Test package path: $testPackagePath"

    # Generate unique test executable name
    $timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
    $testExeName = "test_$timestamp.exe"
    $localTestExe = Join-Path $env:TEMP $testExeName
    $remoteTestExe = "$RemoteWorkDir\$testExeName"

    Write-ColorOutput "Building Go test executable..." "Yellow"

    # Build the test executable
    $buildArgs = @(
        "test",
        "-c",
        "-o", $localTestExe
    )

    if ($BuildTags) {
        $buildArgs += @("-tags", $BuildTags)
        Write-Verbose "Using build tags: $BuildTags"
    }

    # Set environment variables for Windows cross-compilation
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "1"

    # Add test package
    $buildArgs += $TestPackage

    Write-Verbose "Go build command: go $($buildArgs -join ' ')"

    & go @buildArgs
    $exitCode = $LASTEXITCODE

    if ($exitCode -ne 0) {
        throw "Go build failed with exit code $exitCode"
    }

    if (-not (Test-Path $localTestExe)) {
        throw "Test executable was not created: $localTestExe"
    }

    Write-ColorOutput "Test executable built successfully: $localTestExe" "Green"

    # Test SSH connectivity
    Write-ColorOutput "Testing SSH connectivity..." "Yellow"
    Invoke-SSHCommand "Write-Host 'SSH connection successful'"

    # Create remote working directory
    Write-ColorOutput "Creating remote working directory..." "Yellow"
    Invoke-SSHCommand "if (-not (Test-Path '$RemoteWorkDir')) { New-Item -ItemType Directory -Path '$RemoteWorkDir' -Force | Out-Null }"

    # Copy test executable to remote host
    Write-ColorOutput "Copying test executable to remote host..." "Yellow"
    Copy-FileViaSCP $localTestExe $remoteTestExe

    # Run the tests on remote host
    Write-ColorOutput "Running tests on remote host..." "Yellow"

    # Show environment variables if any are set
    if ($EnvVars.Count -gt 0) {
        Write-ColorOutput "Environment variables to set:" "Cyan"
        foreach ($envVar in $EnvVars.GetEnumerator()) {
            Write-ColorOutput "  $($envVar.Key) = $($envVar.Value)" "Gray"
        }
    }

    # Build test command with environment variables
    $testCommand = ""

    # Set environment variables if specified
    if ($EnvVars.Count -gt 0) {
        $envCommands = @()
        foreach ($envVar in $EnvVars.GetEnumerator()) {
            $envCommands += "`$env:$($envVar.Key) = `"$($envVar.Value)`""
        }
        $testCommand += "$($envCommands -join '; '); "
        Write-Verbose "Setting environment variables: $($envCommands -join '; ')"
    }

    # Add the test executable
    $testCommand += "& '$remoteTestExe'"

    # Add timeout parameter if specified
    if ($TestTimeout) {
        $testCommand += " --test.timeout $TestTimeout"
    }

    # Add additional test arguments
    if ($TestArgs) {
        $testCommand += " $TestArgs"
    }

    Write-ColorOutput "Executing test command: $testCommand" "Cyan"

    try {
        Invoke-SSHScriptCommand $testCommand
        Write-ColorOutput "`nTests completed successfully!" "Green"
    } catch {
        Write-ColorOutput "`nTest execution failed:" "Red"
        Write-ColorOutput $_.Exception.Message "Red"
        throw
    }

} catch {
    Write-ColorOutput "`nError: $($_.Exception.Message)" "Red"
    exit 1

} finally {
    # Cleanup
    Write-ColorOutput "`nCleaning up..." "Yellow"

    # Remove local test executable
    if (Test-Path $localTestExe) {
        Remove-Item $localTestExe -Force
        Write-Verbose "Removed local test executable: $localTestExe"
    }

    # Remove remote files if not keeping them
    if (-not $KeepRemoteFiles) {
        try {
            Invoke-SSHCommand "Remove-Item '$remoteTestExe' -Force -ErrorAction SilentlyContinue" "Removing remote executable"
            Write-Verbose "Removed remote test executable: $remoteTestExe"
        } catch {
            Write-ColorOutput "Warning: Could not remove remote test executable" "Yellow"
        }
    } else {
        Write-ColorOutput "Remote files kept at: $RemoteWorkDir" "Cyan"
    }

    # Reset environment variables
    $env:GOOS = $null
    $env:GOARCH = $null
    $env:CGO_ENABLED = $null

    Write-ColorOutput "Cleanup completed." "Green"
}
