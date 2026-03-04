<#
.SYNOPSIS
Generate config schema artifacts.

.DESCRIPTION
Builds the agent binary, generates schema files from it, enriches them with
template comments, and renders example configuration YAML files.

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the
environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

#>
param(
    [bool] $BuildOutOfSource = $false,
    [nullable[bool]] $CheckGoVersion,
    [bool] $InstallDeps = $true
)

. "$PSScriptRoot\common.ps1"

trap {
    Write-Host "trap: $($_.InvocationInfo.Line.Trim()) - $_" -ForegroundColor Yellow
    continue
}

Invoke-BuildScript `
    -BuildOutOfSource $BuildOutOfSource `
    -InstallDeps $InstallDeps `
    -CheckGoVersion $CheckGoVersion `
    -Command {

    # Build the agent binary
    & dda inv -- -e agent.build
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "agent.build failed: $err"
        exit $err
    }

    # Generate base schemas from the agent binary
    & .\bin\agent\agent.exe createschema
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "createschema failed: $err"
        exit $err
    }

    # Enrich the core schema with template comments
    & python pkg\config\schema\parse_template_comment.py pkg\config\config_template.yaml core_schema.yaml core_schema_enriched.yaml
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "parse_template_comment (core) failed: $err"
        exit $err
    }

    # Enrich the system-probe schema with template comments
    & python pkg\config\schema\parse_template_comment.py pkg\config\security-agent_template.yaml system-probe_schema.yaml system-probe_schema_enriched.yaml
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "parse_template_comment (system-probe) failed: $err"
        exit $err
    }

    # Fix/normalise the enriched schemas
    & python pkg\config\schema\fix_schema.py core_schema_enriched.yaml system-probe_schema_enriched.yaml
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "fix_schema.py failed: $err"
        exit $err
    }

    # Render example configuration YAML files
    New-Item -ItemType Directory -Force -Path .\generated_yaml_example | Out-Null
    & go run .\pkg\config\render_config.go .\generated_yaml_example .\pkg\config
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "render_config.go failed: $err"
        exit $err
    }
}
