Param(
    [Parameter(Mandatory=$true)]
    [string] $package,
    [string] $version,
    [string] $omnibusOutput = "$(Get-Location)\omnibus\pkg\",
    # Optional: staging directory used to assemble OCI contents. If not provided, a temp dir is used.
    [string] $stagingDir,
    # Optional: remove the staging directory after packaging. If omitted, defaults to
    # true when the script creates the staging dir, false when user passes -stagingDir.
    [switch] $CleanupStaging
)

$datadogPackagesDir = "C:\devtools\datadog-packages"
$datadogPackageExe = "$datadogPackagesDir\datadog-package.exe"

if (-not (Test-Path $datadogPackageExe -ErrorAction SilentlyContinue)) {
    Write-Host "Downloading datadog-package.exe"
    powershell.exe -Command {
        if ($env:CI_JOB_TOKEN) {
            # CI variable
            $gitlabToken = $env:CI_JOB_TOKEN
        } elseif ($env:GITLAB_TOKEN) {
            # local variable
            $gitlabToken = $env:GITLAB_TOKEN
        } else {
            Write-Error "No GitLab token found, set CI_JOB_TOKEN or GITLAB_TOKEN"
            exit 1
        }
        # Use env var to temporarily override git config just to install datadog-packages, instead of
        # affecting the config for the entire image.
        $env:GIT_CONFIG_PARAMETERS="'url.https://gitlab-ci-token:${gitlabToken}@gitlab.ddbuild.io/DataDog/.insteadOf=https://github.com/DataDog/'"
        go env -w GOPRIVATE="github.com/DataDog/*"
        go install github.com/DataDog/datadog-packages/cmd/datadog-package@latest
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Failed to install datadog-package.exe"
            exit 1
        }
    }
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to install datadog-package.exe"
        exit 1
    }

    New-Item -ItemType Directory $datadogPackagesDir -ErrorAction SilentlyContinue | Out-Null
    Copy-Item "$env:GOPATH\bin\datadog-package.exe" $datadogPackageExe -Force
    $env:PATH += ";$datadogPackagesDir"
}
if ([string]::IsNullOrWhitespace($version)) {
    $version = "{0}-1" -f (dda inv -- agent.version --url-safe --major-version 7)
    Write-Host "Detected agent version ${version}"
}
if (-not $version.EndsWith("-1")) {
    $version += "-1"
}

$packageName = "${package}-${version}-windows-amd64.oci.tar"

if (Test-Path $omnibusOutput\$packageName) {
    Remove-Item $omnibusOutput\$packageName
}

if ([string]::IsNullOrWhiteSpace($stagingDir)) {
    $stagingDir = Join-Path $env:TEMP ("oci-pkg-" + [guid]::NewGuid().ToString())
    $cleanupStaging = $true
} else {
    $cleanupStaging = $false
}

# If the caller explicitly provided -CleanupStaging, do the cleanup
if ($PSBoundParameters.ContainsKey('CleanupStaging')) {
    $cleanupStaging = [bool]$CleanupStaging
}

Remove-Item -Recurse -Force $stagingDir -ErrorAction SilentlyContinue
New-Item -ItemType Directory $stagingDir | Out-Null

if ($package -eq "datadog-agent-ddot") {
    # Build DDOT OCI from locally built otel-agent.exe (fallback to omnibus paths if needed)
    $embeddedBin = Join-Path $stagingDir "embedded\bin"
    $etcDir      = Join-Path $stagingDir "etc\datadog-agent"
    New-Item -ItemType Directory $embeddedBin, $etcDir -Force | Out-Null

    $localExe = Join-Path (Get-Location) "bin\otel-agent\otel-agent.exe"
    $localCfg = Join-Path (Get-Location) "bin\otel-agent\dist\otel-config.yaml"
    $omniExe  = "C:\opt\datadog-agent\embedded\bin\otel-agent.exe"
    $omniCfg  = "C:\opt\datadog-agent\etc\datadog-agent\otel-config.yaml.example"

    if (Test-Path $localExe) {
        Copy-Item $localExe (Join-Path $embeddedBin "otel-agent.exe") -Force
    } elseif (Test-Path $omniExe) {
        Copy-Item $omniExe (Join-Path $embeddedBin "otel-agent.exe") -Force
    } else {
        Write-Error "otel-agent.exe not found. Build it with 'inv otel-agent.build' or provide omnibus output at $omniExe."
        exit 1
    }

    if (Test-Path $localCfg) {
        Copy-Item $localCfg (Join-Path $etcDir "otel-config.yaml.example") -Force
    } elseif (Test-Path $omniCfg) {
        Copy-Item $omniCfg (Join-Path $etcDir "otel-config.yaml.example") -Force
    } else {
        Write-Error "otel-config example not found at $localCfg or $omniCfg"
        exit 1
    }

    Write-Host "Staging tree for ddot: $stagingDir"
    Get-ChildItem -Recurse $stagingDir | Select-Object FullName, Length | Format-Table -AutoSize | Out-Host

    $extraArgs = @("--configs", "$etcDir")
} else {
    # datadog-package takes a folder as input and will package everything in that, so copy the msi to its own folder
    Copy-Item (Get-ChildItem "$omnibusOutput\${package}-${version}-x86_64.msi").FullName -Destination (Join-Path $stagingDir "${package}-${version}-x86_64.msi")
}

$installerPath = "C:\opt\datadog-installer\datadog-installer.exe"
if (Test-Path $installerPath) {
    $installerArg = @("--installer", "`"$installerPath`"")
} else {
    $installerArg = @()
}

# The argument --archive-path ".\omnibus\pkg\datadog-agent-${version}.tar.gz" is currently broken and has no effects
Write-Host "Running: $datadogPackageExe create $installerArg $extraArgs --package $package --os windows --arch amd64 --archive --version $version $stagingDir"
& $datadogPackageExe create @installerArg @extraArgs --package $package --os windows --arch amd64 --archive --version $version $stagingDir
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to create OCI package"
    exit 1
}

if (-not (Test-Path "$omnibusOutput")) {
    New-Item -ItemType Directory "$omnibusOutput" -Force | Out-Null
}

$sourceTar = "${package}-${version}-windows-amd64.tar"
if (-not (Test-Path $sourceTar)) {
    Write-Host "datadog-package output not found: $sourceTar"
    Write-Host "Current directory: $(Get-Location)"
    Write-Host "Directory contents:"
    Get-ChildItem | Format-List -Property Name,Length,LastWriteTime
    Write-Error "Expected tar not found; packaging step may have failed."
    exit 1
}

Move-Item $sourceTar $omnibusOutput\$packageName

try {
    if ($cleanupStaging) {
        Remove-Item -Recurse -Force $stagingDir -ErrorAction SilentlyContinue
    }
} catch {
    Write-Warning "Failed to remove staging directory '$stagingDir': $($_.Exception.Message)"
}
