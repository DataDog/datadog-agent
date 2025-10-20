#
# Measure MSI package size and generate Static Quality Gate report
# This script is called after the MSI build to measure the package size
# and upload the report to S3 for quality gate validation.
#
# Note: This only runs for vanilla builds, not FIPS builds.
#

param(
    [Parameter(Mandatory=$false)]
    [string]$WorkingDirectory = "c:\mnt"
)

$ErrorActionPreference = 'Continue'

# Check if STATIC_QUALITY_GATE_NAME is set
if ($null -eq $env:STATIC_QUALITY_GATE_NAME) {
    Write-Host "ℹ️  Skipping MSI measurement (no STATIC_QUALITY_GATE_NAME defined)"
    exit 0
}

# Check if the gate name matches the build flavor
$IsFipsBuild = $env:AGENT_FLAVOR -eq "fips"
$IsFipsGate = $env:STATIC_QUALITY_GATE_NAME -match "fips"

if ($IsFipsBuild -and -not $IsFipsGate) {
    Write-Host "ℹ️  Skipping MSI measurement for FIPS build (gate '$env:STATIC_QUALITY_GATE_NAME' is not for FIPS)"
    exit 0
}

if (-not $IsFipsBuild -and $IsFipsGate) {
    Write-Host "ℹ️  Skipping MSI measurement for vanilla build (gate '$env:STATIC_QUALITY_GATE_NAME' is for FIPS)"
    exit 0
}

Write-Host "📊 Starting MSI measurement..."

# Determine project name based on AGENT_FLAVOR
if ($IsFipsBuild) {
    $ProjectName = "fips-datadog-agent"
} else {
    $ProjectName = "datadog-agent"
}

# MSI pattern in omnibus package directory
$PackagePattern = "$WorkingDirectory\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID\$ProjectName-7*-x86_64.msi"

Write-Host "🔍 Looking for MSI with pattern: $PackagePattern"

# Extract report prefix from gate name
$ReportPrefix = $env:STATIC_QUALITY_GATE_NAME -replace '^static_quality_gate_', ''

$MsiFiles = Get-ChildItem -Path $PackagePattern -ErrorAction SilentlyContinue

if (-not $MsiFiles) {
    Write-Host "⚠️  No MSI found matching pattern: $PackagePattern"
    exit 0
}

foreach ($MsiFile in $MsiFiles) {
    Write-Host "📏 Measuring MSI: $($MsiFile.FullName)"
    
    # Generate measurement report using STATIC_QUALITY_GATE_NAME variable
    $OutputPath = "$WorkingDirectory\${ReportPrefix}_size_report_${env:CI_PIPELINE_ID}_$($env:CI_COMMIT_SHA.Substring(0,8)).yml"
    
    & "$WorkingDirectory\dda" inv quality-gates.measure-msi `
        --msi-path $MsiFile.FullName `
        --gate-name $env:STATIC_QUALITY_GATE_NAME `
        --build-job-name $env:CI_JOB_NAME `
        --output-path $OutputPath `
        --debug
    
    if ($LASTEXITCODE -ne 0) {
        Write-Host "⚠️  MSI measurement failed for $($MsiFile.FullName)"
        exit 0
    }
    
    Write-Host "✅ MSI measurement completed"
    
    # Upload the report to S3
    $BucketBasePath = "s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates/GATE_REPORTS/$env:CI_COMMIT_SHA"
    Write-Host "📤 Uploading report to ${BucketBasePath}"
    
    try {
        $ReportFileName = Split-Path -Leaf $OutputPath
        aws s3 cp --only-show-errors --region us-east-1 --sse AES256 `
            $OutputPath `
            "${BucketBasePath}/${ReportFileName}"
        
        if ($LASTEXITCODE -ne 0) {
            Write-Host "⚠️  S3 upload failed but continuing"
        } else {
            Write-Host "✅ Report uploaded successfully"
        }
    } catch {
        Write-Host "⚠️  S3 upload failed: $_"
    }
}

Write-Host "🎉 MSI measurement process completed"
exit 0

