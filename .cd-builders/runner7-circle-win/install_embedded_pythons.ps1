#
# Installs embedded python2 and python3 for use in unit testing.
# Be careful to use unique environment variables indicating location
# of the python files.
#
# We need to provide a location so that the test scripts can find python,
# but also need to make sure it doesn't confuse the actual builds.
#

# Uses:
# EMBEDDED_PYTHON_2_VERSION
# EMBEDDED_PYTHON_3_VERSION

# this downloads the necessary file

$ErrorActionPreference = 'Stop'

function DownloadFile{
    param(
        [Parameter(Mandatory = $true)][string] $TargetFile,
        [Parameter(Mandatory = $true)][string] $SourceURL
    )
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

    Write-Host -ForegroundColor Green "Downloading $SourceUrl to $TargetFile"
    (New-Object System.Net.WebClient).DownloadFile($SourceURL, $TargetFile)
}

function DownloadAndExpandTo{
    param(
        [Parameter(Mandatory = $true)][string] $TargetDir,
        [Parameter(Mandatory = $true)][string] $SourceURL
    )
    $tmpOutFile = New-TemporaryFile

    DownloadFile -TargetFile $tmpOutFile -SourceURL $SourceURL

    If(!(Test-Path $TargetDir))
    {
        md $TargetDir
    }

    Start-Process "7z" -ArgumentList "x -o${TargetDir} $tmpOutFile" -Wait
    Remove-Item $tmpOutFile
}

$py2 = "https://s3.amazonaws.com/dd-agent-omnibus/python-windows-${Env:EMBEDDED_PYTHON_2_VERSION}-amd64.zip"
$py3 = "https://s3.amazonaws.com/dd-agent-omnibus/python-windows-${Env:EMBEDDED_PYTHON_3_VERSION}-amd64.zip"

if ( $Env:TARGET_ARCH -eq "x86") {
    $py2 = "https://s3.amazonaws.com/dd-agent-omnibus/python-windows-${Env:EMBEDDED_PYTHON_2_VERSION}-x86.zip"
    $py3 = "https://s3.amazonaws.com/dd-agent-omnibus/python-windows-${Env:EMBEDDED_PYTHON_3_VERSION}-x86.zip"
} 


$py2Target = "c:\embeddedpy\py${Env:EMBEDDED_PYTHON_2_VERSION}"
$py3Target = "c:\embeddedpy\py${Env:EMBEDDED_PYTHON_3_VERSION}"

DownloadAndExpandTo -TargetDir $py2Target -SourceURL $py2
DownloadAndExpandTo -TargetDir $py3Target -SourceURL $py3

setx TEST_EMBEDDED_PY2 $py2Target
setx TEST_EMBEDDED_PY3 $py3Target

cd $py2Target
curl https://bootstrap.pypa.io/get-pip.py -o get-pip.py
.\python get-pip.py


cd $py3Target
curl https://bootstrap.pypa.io/get-pip.py -o get-pip.py
.\python get-pip.py

if ( $Env:TARGET_ARCH -eq "x86") {
    $crt = "https://s3.amazonaws.com/dd-agent-omnibus/msvc_ucrt_runtime_x86.zip"
    DownloadAndExpandTo -TargetDir $py3Target -SourceURL $crt
}