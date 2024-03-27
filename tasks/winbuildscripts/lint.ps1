$ErrorActionPreference = "Stop"

$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

if ($Env:TARGET_ARCH -eq "x64") {
    & ridk enable
}
& $Env:Python3_ROOT_DIR\python.exe -m  pip install -r requirements.txt

$LINT_ROOT=(Get-Location).Path
$Env:PATH="$LINT_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH;$Env:VSTUDIO_ROOT\VC\Tools\Llvm\bin"

$archflag = "x64"
if ($Env:TARGET_ARCH -eq "x86") {
    $archflag = "x86"
}

& inv -e deps
& .\tasks\winbuildscripts\pre-go-build.ps1 -Architecture "$archflag" -PythonRuntimes "$Env:PY_RUNTIMES"

& inv -e rtloader.format --raise-if-changed
$err = $LASTEXITCODE
Write-Host Format result is $err
if($err -ne 0){
  Write-Host -ForegroundColor Red "rtloader format failed $err"
  [Environment]::Exit($err)
}

& inv -e install-tools
& inv -e linter.go --arch $archflag

$err = $LASTEXITCODE
Write-Host Lint result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "lint failed $err"
    [Environment]::Exit($err)
}
