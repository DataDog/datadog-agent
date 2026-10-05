# Normalize $env:GOPROXY on Windows: drop the entry that isn't usable here
# (hosts ending in .fabric.dog), keeping the rest. Mirrors tools/ci/sanitize-goproxy.sh.
#
# Dot-source before `docker run ... -e GOPROXY` so the container inherits the
# cleaned value:  . ./tools/ci/sanitize-goproxy.ps1

$sgpProbeUrl = 'https://depot-read-api-go.rapid-dependency-management-depot.all-clusters.local-dc.fabric.dog:8443/magicmirror/magicmirror/@current/sumdb/sum.golang.org/supported'

function Test-SgpUsable {
    try {
        # -SkipHttpErrorCheck (PS7+) so a non-2xx still counts as completed; any
        # failure throws and is caught below.
        Invoke-WebRequest -Uri $sgpProbeUrl -Method Head -TimeoutSec 5 -SkipHttpErrorCheck -UseBasicParsing | Out-Null
        return $true
    } catch {
        return $false
    }
}

if ($env:GOPROXY -and $env:GOPROXY.Contains('.fabric.dog') -and -not (Test-SgpUsable)) {
    $env:GOPROXY = (($env:GOPROXY -split '\|') |
        Where-Object { $_ -notmatch '^https?://[^/]*\.fabric\.dog(:\d+)?(/|$)' }) -join '|'
    Write-Host "sanitize-goproxy: endpoint unusable here; stripped it from GOPROXY"
}
