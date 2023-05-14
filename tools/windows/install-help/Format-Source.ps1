$ErrorActionPreference = 'Stop'
function Invoke-Process {
    [CmdletBinding(SupportsShouldProcess)]
    param
    (
        [Parameter(Mandatory)]
        [ValidateNotNullOrEmpty()]
        [string]$FilePath,

        [Parameter()]
        [ValidateNotNullOrEmpty()]
        [string]$ArgumentList
    )
    try {
        $stdOutTempFile = "$env:TEMP\$((New-Guid).Guid)"
        $stdErrTempFile = "$env:TEMP\$((New-Guid).Guid)"

        $startProcessParams = @{
            FilePath               = $FilePath
            ArgumentList           = $ArgumentList
            RedirectStandardError  = $stdErrTempFile
            RedirectStandardOutput = $stdOutTempFile
            Wait                   = $true;
            PassThru               = $true;
            NoNewWindow            = $true;
        }
        $cmd = Start-Process @startProcessParams
        $cmdOutput = Get-Content -Path $stdOutTempFile -Raw
        $cmdError = Get-Content -Path $stdErrTempFile -Raw
        if ($cmd.ExitCode -ne 0) {
            if ($cmdError) {
                throw $cmdError.Trim()
            }
            if ($cmdOutput) {
                throw $cmdOutput.Trim()
            }
        } else {
            if ([string]::IsNullOrEmpty($cmdOutput) -eq $false) {
                Write-Output -InputObject $cmdOutput
            }
        }
    } catch {
        $PSCmdlet.ThrowTerminatingError($_)
    } finally {
        Remove-Item -Path $stdOutTempFile, $stdErrTempFile -Force -ErrorAction Ignore
    }
}

function Format-Source {
    Param(
        $clang_format_path,
        $source_dir
    )
    Write-Host ("Formatting code in {0}" -f $source_dir)
    $source_files = [System.Collections.ArrayList]@()
    @("*.h", "*.hpp", "*.cpp") | ForEach-Object {
        Get-ChildItem -Recurse $source_dir -Filter $PSItem | ForEach-Object {
            if (!$_.Directory.FullName.StartsWith("$source_dir\cal\packages")) {
                Write-Host ($_.FullName)
                [void]::($source_files.Add($_.FullName))
            }
        }
    }
    Invoke-Process $clang_format_path ("-i {0}" -f ($source_files -join ' '))
}
$clang_format_config_path = "{0}\.clang-format" -f (Get-Location)
# Make sure to call this from the location where .clang-format is located
if (![System.IO.File]::Exists($clang_format_config_path)) {
    Write-Host ("No .clang-format file found at {0}, using default configuration" -f $clang_format_config_path)
}
Format-Source "C:\Program Files (x86)\Microsoft Visual Studio\2019\Community\vc\Tools\Llvm\bin\clang-format.exe" (Get-Location)