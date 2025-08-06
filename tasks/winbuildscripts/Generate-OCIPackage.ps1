Param(
    [Parameter(Mandatory=$true)]
    [string] $package,
    [string] $version,
    [string] $omnibusOutput = "$(Get-Location)\omnibus\pkg\"
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

# datadog-package takes a folder as input and will package everything in that, so copy the msi to its own folder
Remove-Item -Recurse -Force C:\oci-pkg -ErrorAction SilentlyContinue
New-Item -ItemType Directory C:\oci-pkg | Out-Null
Copy-Item (Get-ChildItem $omnibusOutput\${package}-${version}-x86_64.msi).FullName -Destination C:\oci-pkg\${package}-${version}-x86_64.msi

$installerPath = "C:\opt\datadog-installer\datadog-installer.exe"
if (Test-Path $installerPath) {
    $installerArg = @("--installer", "`"$installerPath`"")
} else {
    $installerArg = @()
}

# The argument --archive-path ".\omnibus\pkg\datadog-agent-${version}.tar.gz" is currently broken and has no effects
Write-Host "Running: $datadogPackageExe create $installerArg --package $package --os windows --arch amd64 --archive --version $version C:\oci-pkg"
& $datadogPackageExe create @installerArg --package $package --os windows --arch amd64 --archive --version $version C:\oci-pkg
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to create OCI package"
    exit 1
}

Move-Item ${package}-${version}-windows-amd64.tar $omnibusOutput\$packageName
