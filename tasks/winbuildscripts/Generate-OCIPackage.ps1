Param(
    [Parameter(Mandatory=$true,Position=0)]
    [ValidateSet("datadog-agent", "datadog-installer")]
    [String]
    $package,
    [Parameter(Mandatory=$false)]
    [String]
    $Version
)

if (-not $Version) {
    $Version = "{0}-1" -f (inv agent.version --url-safe --major-version 7)
}
$omnibusOutput = "$($Env:REPO_ROOT)\omnibus\pkg\"

if (-not (Test-Path C:\tools\datadog-package.exe)) {
    Write-Host "Downloading datadog-package.exe"
    (New-Object System.Net.WebClient).DownloadFile("https://dd-agent-omnibus.s3.amazonaws.com/datadog-package.exe", "C:\\tools\\datadog-package.exe")
}

$packageName = "${package}-${Version}-windows-amd64.oci.tar"

if (Test-Path $omnibusOutput\$packageName) {
    Remove-Item $omnibusOutput\$packageName
}

# datadog-package takes a folder as input and will package everything in that, so copy the msi to its own folder
Remove-Item -Recurse -Force C:\oci-pkg -ErrorAction SilentlyContinue
New-Item -ItemType Directory C:\oci-pkg
Copy-Item (Get-ChildItem $omnibusOutput\${package}-${Version}-x86_64.msi).FullName -Destination C:\oci-pkg\${package}-${Version}-x86_64.msi

# The argument --archive-path ".\omnibus\pkg\datadog-agent-${version}.tar.gz" is currently broken and has no effects
& C:\tools\datadog-package.exe create --package $package --os windows --arch amd64 --archive --version $Version C:\oci-pkg

Move-Item ${package}-${Version}-windows-amd64.tar $omnibusOutput\$packageName
