# Generate periodic outbound TCP and UDP connections so that NPM and
# Network Path dynamic tests have live flows to observe.
#
# Reads target lines from C:\ProgramData\Datadog\conn-gen-targets.txt
# Format: <proto>:<host>:<port>   (proto = tcp|udp)
# Blank lines and lines starting with # are ignored.

$ErrorActionPreference = 'Continue'

$TargetsFile = if ($env:TARGETS_FILE) { $env:TARGETS_FILE } else { 'C:\ProgramData\Datadog\conn-gen-targets.txt' }
$TimeoutSec  = if ($env:CONN_GEN_TIMEOUT) { [int]$env:CONN_GEN_TIMEOUT } else { 5 }

if (-not (Test-Path -LiteralPath $TargetsFile)) {
    Write-Error "conn-gen: targets file $TargetsFile missing or unreadable"
    exit 1
}

function Probe-Tcp {
    param([string]$TargetHost, [int]$Port)
    if ($Port -eq 443) {
        try {
            Invoke-WebRequest -Uri "https://$TargetHost/" -UseBasicParsing -TimeoutSec $TimeoutSec -ErrorAction Stop | Out-Null
            Write-Output "conn-gen: tcp ${TargetHost}:${Port} ok"
        } catch {
            Write-Error "conn-gen: tcp ${TargetHost}:${Port} fail: $($_.Exception.Message)"
        }
        return
    }

    $client = New-Object System.Net.Sockets.TcpClient
    try {
        $task = $client.ConnectAsync($TargetHost, $Port)
        if ($task.Wait($TimeoutSec * 1000) -and $client.Connected) {
            Write-Output "conn-gen: tcp ${TargetHost}:${Port} ok"
        } else {
            Write-Error "conn-gen: tcp ${TargetHost}:${Port} timeout"
        }
    } catch {
        Write-Error "conn-gen: tcp ${TargetHost}:${Port} fail: $($_.Exception.Message)"
    } finally {
        $client.Close()
    }
}

function Probe-Udp {
    param([string]$TargetHost, [int]$Port)
    if ($Port -ne 53) {
        Write-Error "conn-gen: udp ${TargetHost}:${Port} skipped (only DNS-style UDP probes supported)"
        return
    }
    try {
        Resolve-DnsName -Server $TargetHost -Name 'example.com' -Type A -DnsOnly -QuickTimeout -ErrorAction Stop | Out-Null
        Write-Output "conn-gen: udp ${TargetHost}:${Port} ok"
    } catch {
        Write-Error "conn-gen: udp ${TargetHost}:${Port} fail: $($_.Exception.Message)"
    }
}

Get-Content -LiteralPath $TargetsFile | ForEach-Object {
    $line = ($_ -replace '#.*$', '').Trim() -replace '\s+', ''
    if (-not $line) { return }

    $parts = $line.Split(':')
    if ($parts.Count -ne 3 -or -not $parts[0] -or -not $parts[1] -or -not $parts[2]) {
        Write-Error "conn-gen: malformed target '$_'"
        return
    }

    $proto = $parts[0].ToLowerInvariant()
    $targetHost = $parts[1]
    $port = $parts[2]

    switch ($proto) {
        'tcp' { Probe-Tcp -TargetHost $targetHost -Port ([int]$port) }
        'udp' { Probe-Udp -TargetHost $targetHost -Port ([int]$port) }
        default { Write-Error "conn-gen: unknown protocol '$proto' in '$_'" }
    }
}
