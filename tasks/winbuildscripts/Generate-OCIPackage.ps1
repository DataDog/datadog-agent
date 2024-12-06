Param(
    [Parameter(Mandatory=$true)]
    [string] $package,
    [string] $version
)

$omnibusOutput = "$($Env:REPO_ROOT)\omnibus\pkg\"

if (-not (Test-Path C:\tools\datadog-package.exe)) {
    Write-Host "Downloading datadog-package.exe"
    (New-Object System.Net.WebClient).DownloadFile("https://dd-agent-omnibus.s3.amazonaws.com/datadog-package.exe", "C:\\tools\\datadog-package.exe")
}
if ([string]::IsNullOrWhitespace($version)) {
    $version = "{0}-1" -f (inv agent.version --url-safe --major-version 7)
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
New-Item -ItemType Directory C:\oci-pkg
Copy-Item (Get-ChildItem $omnibusOutput\${package}-${version}-x86_64.msi).FullName -Destination C:\oci-pkg\${package}-${version}-x86_64.msi

# The argument --archive-path ".\omnibus\pkg\datadog-agent-${version}.tar.gz" is currently broken and has no effects
& C:\tools\datadog-package.exe create --package $package --os windows --arch amd64 --archive --version $version C:\oci-pkg

Move-Item ${package}-${version}-windows-amd64.tar $omnibusOutput\$packageName
