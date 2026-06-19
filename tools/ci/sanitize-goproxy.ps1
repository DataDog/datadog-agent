# Normalize $env:GOPROXY on Windows: drop the entry that isn't usable here,
# keeping the rest. Mirrors tools/ci/sanitize-goproxy.sh.
#
# Dot-source before `docker run ... -e GOPROXY` so the container inherits the
# cleaned value:  . ./tools/ci/sanitize-goproxy.ps1

$sgpHost     = 'depot-read-api-go.rapid-dependency-management-depot.all-clusters.local-dc.fabric.dog'
$sgpProbeUrl = "https://${sgpHost}:8443/magicmirror/magicmirror/@current/sumdb/sum.golang.org/supported"

function Test-SgpUsable {
    param([string]$Url)
    try {
        # -SkipHttpErrorCheck (PS7+) so a non-2xx still counts as completed; any
        # failure throws and is caught below.
        Invoke-WebRequest -Uri $Url -Method Head -TimeoutSec 5 -SkipHttpErrorCheck -UseBasicParsing | Out-Null
        return $true
    } catch {
        return $false
    }
}

if ($env:GOPROXY -and $env:GOPROXY.Contains('.fabric.dog')) {
    if (Test-SgpUsable -Url $sgpProbeUrl) {
        Write-Host "sanitize-goproxy: endpoint usable here; keeping it in GOPROXY"
    } else {
        $env:GOPROXY = (($env:GOPROXY -split '\|') |
            Where-Object { $_ -notmatch '^https?://[^/]*\.fabric\.dog(:\d+)?(/|$)' }) -join '|'
        Write-Host "sanitize-goproxy: endpoint unusable here; stripped it from GOPROXY"
    }
}
