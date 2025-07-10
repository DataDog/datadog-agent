$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

& ridk enable
$ddaversion = Get-Content -Path ".dda/version" -Raw
& $Env:Python3_ROOT_DIR\python.exe -m pip install "git+https://github.com/DataDog/datadog-agent-dev.git@v$ddaversion"
& $Env:Python3_ROOT_DIR\python.exe -m dda self dep sync -f legacy-tasks

$PROBE_BUILD_ROOT=(Get-Location).Path
$Env:PATH="$PROBE_BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:Python2_ROOT_DIR;$Env:Python2_ROOT_DIR\Scripts;$Env:PATH"

& dda inv -- -e deps
& dda inv -- -e install-tools

# Must build the rtloader libs cgo depends on before running golangci-lint, which requires code to be compilable
& .\tasks\winbuildscripts\pre-go-build.ps1 -PythonRuntimes "$Env:PY_RUNTIMES"

& dda inv -- -e linter.go --build system-probe-unit-tests --targets .\pkg
$err = $LASTEXITCODE
if ($err -ne 0) {
    Write-Host -ForegroundColor Red "linter.go failed $err"
    [Environment]::Exit($err)
}

& dda inv -- -e system-probe.e2e-prepare --ci

$err = $LASTEXITCODE
if($err -ne 0){
    Write-Host -ForegroundColor Red "e2e prepare failed $err"
    [Environment]::Exit($err)
}
Write-Host Test passed
