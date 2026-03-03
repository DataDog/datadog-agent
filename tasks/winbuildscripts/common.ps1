<#
.SYNOPSIS
Copies files from C:\mnt into C:\buildroot\datadog-agent and sets the current directory to the buildroot.

.PARAMETER buildroot
Specifies the root directory where the files will be copied to. The default value is "c:\buildroot".

.NOTES
This function is typically used in the context of building and running scripts within a containerized environment to
ensure that the files are copied to the container filesystem before running the build scripts. This is useful for
keeping the source clean and can provide speed improvements for hyper-v based containers.
See also, issue with job cross-contamination due to improperly cancelled jobs: https://datadoghq.atlassian.net/browse/CIEXE-143
#>
function Enter-BuildRoot() {
    param(
        [string] $buildroot = "c:\buildroot"
    )
    if (-Not (Test-Path -Path "c:\mnt")) {
        Write-Error "C:\mnt directory not mounted, parameters incorrect"
        exit 1
    }

    # copy to buildroot
    mkdir -Force "$buildroot\datadog-agent" -ErrorAction Stop | Out-Null
    if (-Not (Test-Path -Path "$buildroot\datadog-agent")) {
        Write-Error "Failed to create buildroot directory"
        exit 2
    }

    # copy the repository into the container filesystem
    Write-Host "Switching to buildroot $buildroot\datadog-agent"
    Push-Location "$buildroot\datadog-agent" -ErrorAction Stop -StackName AgentBuildRoot
    xcopy /e/s/h/q c:\mnt\*.*
}

<#
.SYNOPSIS
Leaves the buildroot directory and returns to the original working directory.
#>
function Exit-BuildRoot() {
    Write-Host "Leaving buildroot"
    Pop-Location -StackName AgentBuildRoot
}

<#
.SYNOPSIS
Sets the current directory to the root of the repository.
#>
function Enter-RepoRoot() {
    # Expected PSScriptRoot: datadog-agent\tasks\winbuildscripts\
    Push-Location "$PSScriptRoot\..\.." -ErrorAction Stop -StackName AgentRepoRoot | Out-Null
}

<#
.SYNOPSIS
Leaves the repository root directory and returns to the original working directory.
#>
function Exit-RepoRoot() {
    Pop-Location -StackName AgentRepoRoot
}

<#
.SYNOPSIS
Expands the Go module cache from an archive file.

.DESCRIPTION
This function expands the Go module cache from an archive file located in the specified root directory.
It extracts the contents of the archive file into the Go module cache directory defined by the GOMODCACHE environment variable.

.PARAMETER root
The root directory where the tar.xz file is located. Defaults to the current location.

.PARAMETER modcache
The base name (without extension) of the file to be expanded. Expected values are `modcache` and `modcache_tools`.

.NOTES
If the GOMODCACHE environment variable is not set, the function will skip the expansion process.

.EXAMPLE
Expand-ModCache -modcache "modcache"
This will expand the modcache file located at "<CWD>\modcache.tar.xz" into the Go module cache directory.

#>
function Expand-ModCache() {
    param(
        [string] $root = (Get-Location).Path,
        [ValidateSet('modcache', 'modcache_tools')]
        [string] $modcache
    )

    $MODCACHE_ROOT = $root
    $MODCACHE_FILE_ROOT = $modcache
    $MODCACHE_XZ_FILE = Join-Path $MODCACHE_ROOT "$MODCACHE_FILE_ROOT.tar.xz"
    $MODCACHE_TAR_FILE = Join-Path $MODCACHE_ROOT "$MODCACHE_FILE_ROOT.tar"

    if (-not $env:GOMODCACHE) {
        Write-Host "GOMODCACHE environment variable not set, skipping expansion of mod cache files"
        return
    }

    Write-Host "MODCACHE_XZ_FILE $MODCACHE_XZ_FILE MODCACHE_TAR_FILE $MODCACHE_TAR_FILE GOMODCACHE $env:GOMODCACHE"
    if (Test-Path $MODCACHE_XZ_FILE) {
        Write-Host "Extracting modcache file $MODCACHE_XZ_FILE"
        & 7z.exe x $MODCACHE_XZ_FILE -o"$MODCACHE_ROOT" -bt
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Failed to extract $MODCACHE_XZ_FILE"
            exit 1
        }
        Get-ChildItem $MODCACHE_TAR_FILE
        # Use -aoa to allow overwriting existing files
        # This shouldn't have any negative impact: since modules are
        # stored per version and hash, files that get replaced will
        # get replaced by the same files
        & 7z.exe x $MODCACHE_TAR_FILE -o"$env:GOMODCACHE\cache" -aoa -bt
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Failed to extract $MODCACHE_XZ_FILE"
            exit 1
        }
        Write-Host "Modcache extracted"
    } else {
        Write-Host "Modcache XZ file $MODCACHE_XZ_FILE not found, dependencies will be downloaded"
    }

    if (Test-Path $MODCACHE_XZ_FILE) {
        Write-Host "Deleting modcache tar.xz $MODCACHE_XZ_FILE"
        Remove-Item -Force $MODCACHE_XZ_FILE
    }
    if (Test-Path $MODCACHE_TAR_FILE) {
        Write-Host "Deleting modcache tar $MODCACHE_TAR_FILE"
        Remove-Item -Force $MODCACHE_TAR_FILE
    }
}

function Install-Deps() {
    Write-Host "Installing python requirements"
    pip3.exe install dda
    dda self dep sync -f legacy-build
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to install python requirements"
        exit 1
    }
    # Note on Go dependencies
    # CI: downloaded in the CI via the modcache archive, see Expand-ModCache
    # Locally: go automatically downloads deps when running go build/test.
    #          if you want to pre-download everything, see `dda inv deps`
}

function Install-TestingDeps() {
    Write-Host "Installing testing dependencies"
    dda inv -- -e install-tools
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to install testing dependencies"
        exit 1
    }
}

function Enable-DevEnv() {
    # Add go bin to PATH for golangci-lint and other go tools
    if (-Not $env:GOPATH) {
        Write-Host "GOPATH not set, setting to C:\dev\go"
        $env:GOPATH = "C:\dev\go"
    }
    $env:PATH = "$env:GOPATH\bin;$env:PATH"

    # Add clang-format to PATH for rtloader linting
    if (-Not (Get-Command clang-format.exe -ErrorAction SilentlyContinue)) {
        # Included by default in Visual Studio
        $env:PATH = "$(Get-VisualStudioRoot)\VC\Tools\Llvm\bin;$env:PATH"
    }

    # Enable ruby/msys environment, for mingw, make, etc.
    ridk enable
}

function Get-VisualStudioRoot() {
    # VSINSTALLDIR is set in the Visual Studio Developer Command Prompt
    if (![string]::IsNullOrEmpty($env:VSINSTALLDIR)) {
        return $env:VSINSTALLDIR
    }
    # VSTUDIO_ROOT is set in build container
    if (![string]::IsNullOrEmpty($env:VSTUDIO_ROOT)) {
        return $env:VSTUDIO_ROOT
    }
    # Return a reasonable default
    return "C:\Program Files\Microsoft Visual Studio\2022\Professional"
}

<#
.SYNOPSIS
Fetches a secret

.PARAMETER parameterName
The name of the secret to fetch

.PARAMETER parameterField
The field of the secret to fetch. Only used with vault secrets.

.EXAMPLE
$Env:CODECOV_TOKEN=$(Get-VaultSecret -parameterName "$Env:CODECOV_TOKEN")

Fetch a secret and store it in an environment variable

#>
function Get-VaultSecret() {
    param(
        [string]$parameterName,
        [string]$parameterField
    )
    $tmpFile = [System.IO.Path]::GetTempFileName()
    try {
        # Use Out-Null to suppress the output of the fetch_secret script
        & "$PSScriptRoot\..\..\tools\ci\fetch_secret.ps1" -parameterName $parameterName -tempFile "$tmpfile" | Out-Null
        $err = $LASTEXITCODE
        If ($LASTEXITCODE -ne "0") {
            throw "Failed to fetch ${parameterName}: $err"
        }
        Get-Content -Encoding ASCII -Raw -Path $tmpFile -ErrorAction Stop
    } finally {
        Remove-Item -Force $tmpFile -ErrorAction Continue | Out-Null
    }
}

<#
.SYNOPSIS
Converts a string value to a boolean value based on specific conditions.

.DESCRIPTION
This function takes a string input and a default boolean value.
- If the input string is null or empty, it returns the default boolean value.
- If the input string is "true", "yes", or "1" (case insensitive), it returns $true.
- Otherwise, it returns $false.
#>
function Convert-StringToBool() {
    param(
        [string] $Value,
        [bool] $DefaultValue
    )

    if ([string]::IsNullOrEmpty($Value)) {
        return $DefaultValue
    }

    if ($Value.ToLower() -eq "true") {
        return $true
    }

    if ($Value.ToLower() -eq "yes" -or $Value -eq "1") {
        return $true
    }

    return $false
}

<#
.SYNOPSIS
Invokes a build script with optional parameters for build environment configuration.

.DESCRIPTION
The Invoke-BuildScript function sets up the build environment, optionally installs dependencies, checks the Go version, and executes a provided script block. It supports building out of source and restores the original working directory upon completion.

.PARAMETER buildroot
Specifies the root directory for the build. Default is "c:\buildroot".

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

.PARAMETER Command
A script block containing the commands to execute as part of the build process.

.EXAMPLE
Invoke-BuildScript -buildroot "c:\mybuild" -BuildOutOfSource $true -Command { ./build.ps1 }

#>
function Invoke-BuildScript {
    param(
        [string] $buildroot = "c:\buildroot",
        [bool] $BuildOutOfSource = $false,
        [bool] $InstallDeps = $true,
        [bool] $InstallTestingDeps = $false,
        [nullable[bool]] $CheckGoVersion,
        [ScriptBlock] $Command = {$null}
    )

    try {
        if ($null -eq $CheckGoVersion) {
            $CheckGoVersion = Convert-StringToBool -Value $env:GO_VERSION_CHECK -default $true
        }

        if ($BuildOutOfSource) {
            # Expand modcache from c:\mnt to avoid needlessly xcopy'ing large tarballs
            if ($InstallDeps) {
                Expand-ModCache -root "c:\mnt" -modcache modcache
            }
            if ($InstallTestingDeps) {
                Expand-ModCache -root "c:\mnt" -modcache modcache_tools
            }
            Enter-BuildRoot
        } else {
            Enter-RepoRoot
            # Expand modcache from current directory otherwise
            if ($InstallDeps) {
                Expand-ModCache -modcache modcache
            }
            if ($InstallTestingDeps) {
                Expand-ModCache -modcache modcache_tools
            }
        }

        Enable-DevEnv

        # Initialize CI identity if running in CI environment
        if ($env:CI) {
            Initialize-CIIdentity
        }

        # Install deps
        if ($InstallDeps) {
            Install-Deps
        }
        if ($InstallTestingDeps) {
            Install-TestingDeps
        }

        if ($CheckGoVersion) {
            dda inv -- -e check-go-version
            if ($LASTEXITCODE -ne 0) {
                Write-Error "Go version check failed"
                exit 1
            }
        }

        # Execute the provided ScriptBlock/Command
        & $Command

    } finally {
        # This finally block is executed regardless of whether the try block completes successfully, throws an exception,
        # or uses `exit` to terminate the script.

        # Restore the original working directory
        if ($BuildOutOfSource) {
            Exit-BuildRoot
        } else {
            Exit-RepoRoot
        }
    }
}

<#
.SYNOPSIS
Downloads the CI identity client and assumes the CI Identity IAM role.

.DESCRIPTION
This function downloads the CI identity client from S3 and uses it to assume the CI Identity IAM role.
It is typically called in CI environments to authenticate with AWS services.

.NOTES
This function requires AWS CLI to be available and properly configured.
#>
function Initialize-CIIdentity() {
    Write-Host "Assuming CI role..."
    C:\devtools\ci-identities-gitlab-job-client.exe assume-role
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to assume CI role (exit code: $LASTEXITCODE)"
        exit 1
    }
}

