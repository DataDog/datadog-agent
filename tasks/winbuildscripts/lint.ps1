$ErrorActionPreference = "Stop"

$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

& ridk enable
& $Env:Python3_ROOT_DIR\python.exe -m  pip install -r requirements.txt

$LINT_ROOT=(Get-Location).Path
$Env:PATH="$LINT_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH;$Env:VSTUDIO_ROOT\VC\Tools\Llvm\bin"

& inv -e deps
& .\tasks\winbuildscripts\pre-go-build.ps1

& inv -e rtloader.format --raise-if-changed
$err = $LASTEXITCODE
Write-Host Format result is $err
if($err -ne 0){
  Write-Host -ForegroundColor Red "rtloader format failed $err"
  [Environment]::Exit($err)
}

& inv -e install-tools

& inv -e linter.go --debug
$err = $LASTEXITCODE
Write-Host Go linter result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "go linter failed $err"
    [Environment]::Exit($err)
}

$timeTaken = Measure-Command {
  & dotnet format --verify-no-changes .\\tools\\windows\\DatadogAgentInstaller
  $err = $LASTEXITCODE
  Write-Host Dotnet linter result is $err
  if($err -ne 0){
      Write-Host -ForegroundColor Red "dotnet linter failed $err"
      [Environment]::Exit($err)
  }
}

Write-Host "Dotnet linter run time: $($timeTaken.TotalSeconds) seconds"
