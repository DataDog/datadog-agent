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
      $oldPath=[Environment]::GetEnvironmentVariable("Path", [System.EnvironmentVariableTarget]::User)
      $target="$oldPath;$NewPath"
        [Environment]::SetEnvironmentVariable("Path", $target, [System.EnvironmentVariableTarget]::User)
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

function DownloadAndExpandTo{
    param(
        [Parameter(Mandatory = $true)][string] $TargetDir,
        [Parameter(Mandatory = $true)][string] $SourceURL,
        [Parameter(Mandatory = $true)][string] $Sha256
    )
    $tmpOutFile = New-TemporaryFile

    Get-RemoteFile -LocalFile $tmpOutFile -RemoteFile $SourceURL -VerifyHash $Sha256

    If(!(Test-Path $TargetDir))
    {
        md $TargetDir
    }

    Start-Process "7z" -ArgumentList "x -o${TargetDir} $tmpOutFile" -Wait
    Remove-Item $tmpOutFile
}



# Crane download URL and installation path
$craneUrl = "https://github.com/google/go-containerregistry/releases/download/v0.20.3/go-containerregistry_Windows_x86_64.tar.gz"
$installPath = "C:\crane"

# Download and extract Crane
DownloadAndExpandTo -TargetDir $installPath -SourceURL $craneUrl -Sha256 "939c63961fc2e9d7f0cc2b6a1af9d17a5b2f6a37ffb63d96"

# Add to system PATH
Write-Host "Adding Crane to system PATH..."
Add-ToPath -NewPath $installPath -Global

Write-Host "Crane installed successfully in $installPath"
Write-Host "You may need to restart your terminal for the PATH changes to take effect."

# Verify installation
$exePath = "$installPath\crane.exe"
$craneVersion = & $exePath version
Write-Host "Installed Crane version: $craneVersion"
