<#
.SYNOPSIS
Generate the config schema artifacts and validate that schema-based template generation
matches the legacy Go template generation.

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

#>
param(
    [bool] $BuildOutOfSource = $false,
    [nullable[bool]] $CheckGoVersion,
    [bool] $InstallDeps = $true
)

. "$PSScriptRoot\common.ps1"

Invoke-BuildScript `
    -BuildOutOfSource $BuildOutOfSource `
    -InstallDeps $InstallDeps `
    -CheckGoVersion $CheckGoVersion `
    -Command {

    & dda inv -- -e agent.build
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "Agent build failed $err"
        exit $err
    }

    & dda inv -- schema.generate --agent-bin=.\bin\agent\agent.exe
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "Schema generation failed $err"
        exit $err
    }

    # Copy the schema files to c:\mnt so they are available as CI artifacts
    # even if the template comparison below fails.
    $schemaDir = Join-Path (Get-Location) "pkg\config\schema"
    $mntSchemaDir = "c:\mnt\pkg\config\schema"
    New-Item -ItemType Directory -Force -Path $mntSchemaDir | Out-Null
    Copy-Item -Path "$schemaDir\*" -Destination $mntSchemaDir -Force

    # Generate templates using the new schema-based approach.
    $newTemplatesDir = Join-Path (Get-Location) "new_templates_tmp"
    New-Item -ItemType Directory -Force -Path $newTemplatesDir | Out-Null
    & dda inv -- schema.template-all "$schemaDir\core_schema.yaml" "$schemaDir\system-probe_schema.yaml" $newTemplatesDir
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "New template generation failed $err"
        exit $err
    }

    # Generate templates using the old Go template approach.
    $oldTemplatesDir = Join-Path (Get-Location) "old_templates_tmp"
    New-Item -ItemType Directory -Force -Path $oldTemplatesDir | Out-Null
    & go run .\pkg\config\render_config\render_config.go $oldTemplatesDir .\pkg\config
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "Old template generation failed $err"
        exit $err
    }

    # Compare the two sets of generated templates.
    $failed = $false

    $newFiles = Get-ChildItem -Path $newTemplatesDir -Filter "*.yaml" | Sort-Object Name
    $oldFiles = Get-ChildItem -Path $oldTemplatesDir -Filter "*.yaml" | Sort-Object Name

    if ($newFiles.Count -ne $oldFiles.Count) {
        Write-Host "ERROR: Different number of generated files: new=$($newFiles.Count), old=$($oldFiles.Count)"
        Write-Host "New files: $($newFiles.Name -join ', ')"
        Write-Host "Old files: $($oldFiles.Name -join ', ')"
        $failed = $true
    }

    $newNames = $newFiles | Select-Object -ExpandProperty Name
    $oldNames = $oldFiles | Select-Object -ExpandProperty Name
    $nameDiff = Compare-Object $newNames $oldNames
    if ($nameDiff) {
        Write-Host "ERROR: File names don't match between new and old templates"
        Write-Host "Only in new: $(($nameDiff | Where-Object SideIndicator -eq '<=' | Select-Object -ExpandProperty InputObject) -join ', ')"
        Write-Host "Only in old: $(($nameDiff | Where-Object SideIndicator -eq '=>' | Select-Object -ExpandProperty InputObject) -join ', ')"
        $failed = $true
    }

    foreach ($newFile in $newFiles) {
        $oldFile = Get-Item -Path (Join-Path $oldTemplatesDir $newFile.Name) -ErrorAction SilentlyContinue
        if (-not $oldFile) {
            continue
        }

        $newContent = (Get-Content $newFile.FullName) | ForEach-Object { $_.TrimEnd() }
        $oldContent = (Get-Content $oldFile.FullName) | ForEach-Object { $_.TrimEnd() }

        $maxLines = [Math]::Max($newContent.Count, $oldContent.Count)
        $diffLines = @()
        for ($i = 0; $i -lt $maxLines; $i++) {
            $newLine = if ($i -lt $newContent.Count) { $newContent[$i] } else { "<missing>" }
            $oldLine = if ($i -lt $oldContent.Count) { $oldContent[$i] } else { "<missing>" }
            if ($newLine -ne $oldLine) {
                $diffLines += "Line $($i + 1):"
                $diffLines += "  < $newLine"
                $diffLines += "  > $oldLine"
            }
        }

        if ($diffLines.Count -gt 0) {
            Write-Host "ERROR: $($newFile.Name) differs between new (schema-based) and old (template-based) generation:"
            $diffLines | ForEach-Object { Write-Host $_ }
            Write-Host "---"
            $failed = $true
        }
    }

    if ($failed) {
        Write-Host ""
        Get-Content (Join-Path (Get-Location) "tasks\schema\template_diff_failure_message.txt") | ForEach-Object { Write-Host $_ }
        exit 1
    }
}
