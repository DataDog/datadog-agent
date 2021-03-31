param (
    [Parameter(Mandatory=$true)][string]$Version,
    [Parameter(Mandatory=$false)][string]$md5sum,
    [Parameter(Mandatory=$false)][string]$OutDir,
    [Parameter(Mandatory=$false)][switch]$x86
)
# https://www.python.org/ftp/python/2.7.17/python-2.7.17.amd64.msi
# https://www.python.org/ftp/python/2.7.17/python-2.7.17.msi

# https://www.python.org/ftp/python/3.8.0/python-3.8.0-amd64.exe
# https://www.python.org/ftp/python/3.8.0/python-3.8.0.exe

$maj, $min, $patch = $Version.Split(".")
Write-Host -ForegroundColor Green "Installing Python version $maj $min $patch"

if($maj -eq "2") {
    $url = "https://www.python.org/ftp/python/$Version/python-$Version.amd64.msi"
} elseif ($maj -eq "3"){
    $url = "https://www.python.org/ftp/python/$Version/python-$Version-amd64.exe"
} else {
    Write-Host -ForegroundColor Red "Unknown major version $Maj.  I don't know how to do this"
    exit 1
}
$outzip = "python-windows-$Version-x64.zip"
if($x86 -eq $true){
    if($maj -eq "2") {
        $url = "https://www.python.org/ftp/python/$Version/python-$Version.msi"
    } elseif ($maj -eq "3"){
        $url = "https://www.python.org/ftp/python/$Version/python-$Version.exe"
    }
    $outzip = "python-windows-$Version-x86.zip"
}
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$dlfilename = ($url -split '/')[-1]
$dlfullpath = "$Env:TEMP\$dlfilename"
Write-Host -ForegroundColor Green "Downloading file from $url..."
(New-Object System.Net.WebClient).DownloadFile($url, $dlfullpath)
Write-Host -ForegroundColor Green "... Done."


## do md5 check if we have it
if($md5sum){
    $downloadedhash = (Get-FileHash -Algorithm md5 $dlfullpath).Hash.ToLower()
    if($downloadedhash -ne $md5sum.ToLower()){
        Write-Host -ForegroundColor Red "MD5Sums Don't Match"
        exit -1
    }
    Write-Host -ForegroundColor Green "Matched MD5 Sums"
} else {
    Write-Host -ForegroundColor Yellow "MD5 Hash not supplied, not comparing"
}

Write-Host -ForegroundColor Green "Installing package in container..."
if($maj -eq "2") {
    $p = Start-Process msiexec -ArgumentList "/q /i $dlfullpath TARGETDIR=c:\pythonroot ADDLOCAL=DefaultFeature,Tools,TclTk,Testsuite" -Wait -Passthru

    if ($p.ExitCode -ne 0) {
        Write-Host -ForegroundColor Red ("Failed to install Python, exit code 0x{0:X}" -f $p.ExitCode)
        exit 1
    }
}
elseif ($maj -eq "3") {
    $Env:PATH="$Env:PATH;c:\tools\msys64\mingw64\bin"
    $p = Start-Process $dlfullpath -ArgumentList "/quiet DefaultJustForMeTargetDir=c:\pythonroot TARGETDIR=c:\pythonroot AssociateFiles=0 Include_doc=0 Include_launcher=0 Include_pip=0 Include_tcltk=0" -Wait -Passthru

    if ($p.ExitCode -ne 0) {
        Write-Host -ForegroundColor Red ("Failed to install Python, exit code 0x{0:X}" -f $p.ExitCode)
        exit 1
    }

if (Test-Path c:\pythonroot\vcruntime*.dll) {
        Write-Host -ForegroundColor Green "NOT deleting included cruntime"
    }
    Write-Host -ForegroundColor Green "Generating import library"
    pushd .
    cd \pythonroot\libs
    gendef ..\python3$($min).dll
    dlltool --dllname python3$($min).dll --def python3$($min).def --output-lib libpython3$($min).a
    popd
}
Write-Host -ForegroundColor Green "... Done."

Write-Host -ForegroundColor Green "Zipping results..."
& 7z a -r c:\tmp\$outzip c:\pythonroot\*.*
Write-Host -ForegroundColor Green "... Done."

Write-Host -ForegroundColor Green "Cleaning up (uninstalling)"
if($maj -eq "2") {
    Start-Process msiexec -ArgumentList "/q /x $dlfullpath" -Wait
} else {
    Write-Host -ForegroundColor Yellow "Not uninstalling python3"
}
Remove-Item $dlfullpath
Write-Host -ForegroundColor Green "... Done."
if (Test-Path c:\pythonroot) {
    Write-Host -ForegroundColor Green "Cleaning up remnant files..."
    Remove-Item -Recurse -Force c:\pythonroot
    Write-Host -ForegroundColor Green "... Done."
}

if($OutDir -and (Test-Path $OutDir)){
    Write-Host -ForegroundColor green "Copying zip c:\tmp\$($outzip)to $OutDir"
    Copy-Item -Path "c:\tmp\$($outzip)" -Destination "$($OutDir)"
    Write-Host -ForegroundColor Green "... Done."
}
$shasum = (Get-FileHash "c:\tmp\$($outzip)").Hash.ToLower()
Write-Host -ForegroundColor Green "SHA256 Sum of resulting zip: $shasum"

