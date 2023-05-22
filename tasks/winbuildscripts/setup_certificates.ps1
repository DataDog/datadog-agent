# Taken from https://github.com/actions/runner-images/blob/59997be01ae4da61faedbb48aa5d57f5a5e391c3/images/win/scripts/Installers/Install-RootCA.ps1
# MIT License
# Copyright (c) 2020 GitHub

function Invoke-WithRetry {
     <#
        .SYNOPSIS
        Runs $command block until $BreakCondition or $RetryCount is reached.
     #>

     param([ScriptBlock]$Command, [ScriptBlock] $BreakCondition, [int] $RetryCount=5, [int] $Sleep=10)

     $c = 0
     while($c -lt $RetryCount){
        $result = & $Command
        if(& $BreakCondition){
            break
        }
        Start-Sleep $Sleep
        $c++
     }
     $result
}

function Import-SSTFromWU {
    # Serialized Certificate Store File
    $sstFile = "$env:TEMP\roots.sst"
    # Generate SST from Windows Update
    $result = Invoke-WithRetry { certutil.exe -generateSSTFromWU $sstFile } {$LASTEXITCODE -eq 0}
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[Error]: failed to generate $sstFile sst file`n$result"
        exit $LASTEXITCODE
    }

    $result = certutil.exe -dump $sstFile
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[Error]: failed to dump $sstFile sst file`n$result"
        exit $LASTEXITCODE
    }

    try {
        Import-Certificate -FilePath $sstFile -CertStoreLocation Cert:\LocalMachine\Root
    } catch {
        Write-Host "[Error]: failed to import ROOT CA`n$_"
        exit 1
    }
}

Write-Host "Importing certificates from Windows Update"
Import-SSTFromWU
Write-Host "Imported certificates from Windows Update"
