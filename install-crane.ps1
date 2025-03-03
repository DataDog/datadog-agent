function Add-ToPath() {
    param(
        [Parameter(Mandatory = $true)][string] $NewPath,
        [Parameter(Mandatory = $false)][switch] $Local,
        [Parameter(Mandatory = $false)][switch] $Global
    )
    if($Local) {
        if( $NewPath -like "*python*"){
            $Env:Path="$NewPath;$Env:PATH"
        } else {
            $Env:Path="$Env:Path;$NewPath"
        }
    }
    if($Global){
        if($TargetContainer){
            $oldPath=[Environment]::GetEnvironmentVariable("Path", [System.EnvironmentVariableTarget]::User)
            $target="$oldPath;$NewPath"
            [Environment]::SetEnvironmentVariable("Path", $target, [System.EnvironmentVariableTarget]::User)
        } else {
            if ($GlobalEnvVariables.PathEntries -notcontains $NewPath){
                $GlobalEnvVariables.PathEntries += $NewPath
            }
        }
    }
}

function Get-RemoteFile() {
    param(
        [Parameter(Mandatory = $true)][string] $RemoteFile,
        [Parameter(Mandatory = $true)][string] $LocalFile,
        [Parameter(Mandatory = $false)][string] $VerifyHash
    )
    Write-Host -ForegroundColor Green "Downloading: $RemoteFile"
    Write-Host -ForegroundColor Green "         To: $LocalFile"
    (New-Object System.Net.WebClient).DownloadFile($RemoteFile, $LocalFile)
    if ($PSBoundParameters.ContainsKey("VerifyHash")){
        $dlhash = (Get-FileHash -Algorithm SHA256 $LocalFile).hash.ToLower()
        if($dlhash -ne $VerifyHash){
            Write-Host -ForegroundColor Red "Unexpected file hash downloading $LocalFile from $RemoteFile"
            Write-Host -ForegroundColor Red "Expected $VerifyHash, got $dlhash"
            throw 'Unexpected File Hash'
        }
    }
}


# Crane download URL and installation path
$craneUrl = "https://github.com/google/go-containerregistry/releases/download/v0.20.3/go-containerregistry_Windows_x86_64.tar.gz"
$installPath = "C:\crane"
$archivePath = "$installPath\crane.tar.gz"
$exePath = "$installPath\crane.exe"

# Ensure installation directory exists
if (!(Test-Path -Path $installPath)) {
    New-Item -ItemType Directory -Path $installPath -Force | Out-Null
}

# Download Crane tar.gz file
Write-Host "Downloading Crane..."
Get-RemoteFile -RemoteFile $craneUrl -LocalFile $archivePath -VerifyHash "939c63961fc2e9d7f0cc2b6a1af9d17a5b2f6a37ffb63d961b47f786aadb732b"

# Extract the tar.gz file
Write-Host "Extracting Crane..."
tar -xzf $archivePath -C $installPath

# Rename extracted file if necessary
$extractedExe = Get-ChildItem -Path $installPath -Filter "crane*.exe" | Select-Object -ExpandProperty FullName
if ($extractedExe -and ($extractedExe -ne $exePath)) {
    Rename-Item -Path $extractedExe -NewName "crane.exe" -Force
}

# Clean up archive file
Remove-Item -Path $archivePath -Force

# Add to system PATH
Write-Host "Adding Crane to system PATH..."
Add-ToPath -NewPath $installPath -Global

Write-Host "Crane installed successfully in $installPath"
Write-Host "You may need to restart your terminal for the PATH changes to take effect."

# Verify installation
$craneVersion = & $exePath version
Write-Host "Installed Crane version: $craneVersion"
