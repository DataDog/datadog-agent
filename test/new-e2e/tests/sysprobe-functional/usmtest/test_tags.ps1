

param(
    [Parameter(Mandatory=$true)][string]$TargetHost,
    [Parameter(Mandatory=$true)][string]$TargetPort,
    [Parameter(Mandatory=$true)][string]$TargetPath,
    [Parameter(Mandatory=$true)][string[]]$ExpectedClientTags,
    [Parameter(Mandatory=$true)][string[]]$ExpectedServerTags,
    [Parameter(Mandatory=$true)][string]$ConnExe
)
# dd-procmgr supervises process-agent and system-probe, so their legacy SCM services are
# stopped and starting or stopping them would not touch the running processes.
$installPath = (Get-ItemProperty -Path 'HKLM:\SOFTWARE\Datadog\Datadog Agent' -Name InstallPath).InstallPath
$procmgr = Join-Path $installPath 'bin\agent\dd-procmgr.exe'

# The harness runs this script with $ErrorActionPreference='Stop', which turns anything a
# native command writes to stderr into a terminating error. dd-procmgr reports failures there,
# so its exit code is the only signal worth acting on and the preference has to be relaxed for
# the call itself.
function invoke-procmgr {
    param([Parameter(Mandatory=$true)][string[]]$ProcmgrArgs)
    $ErrorActionPreference = 'Continue'
    $out = & $procmgr @ProcmgrArgs 2>&1
    return [pscustomobject]@{ ExitCode = $LASTEXITCODE; Output = $out }
}

# Returns $null when the state could not be read, which callers treat as "not converged yet"
# rather than as a state of its own: a failed describe must never pass for a stopped process.
function get-procmgrstate {
    param([Parameter(Mandatory=$true)][string]$Name)
    $result = invoke-procmgr -ProcmgrArgs @("describe", $Name)
    if ($result.ExitCode -ne 0) {
        Write-Host -ForegroundColor Yellow "dd-procmgr describe $Name failed: $($result.Output -join ' ')"
        return $null
    }
    foreach ($line in @($result.Output)) {
        if ($line -match '^\s*State:\s*(\S+)') {
            return $Matches[1]
        }
    }
    Write-Host -ForegroundColor Yellow "dd-procmgr describe $Name reported no state: $($result.Output -join ' ')"
    return $null
}

# Drives $Name into one of $ReachedStates, re-issuing "dd-procmgr $Verb" whenever the current
# state calls for it. Requests go through a local gRPC channel that can abort mid-call, so a
# single failed request is not fatal as long as the state converges. States in neither list are
# transitions in flight and are simply waited out.
function converge-procmgrstate {
    param(
        [Parameter(Mandatory=$true)][string]$Name,
        [Parameter(Mandatory=$true)][string]$Verb,
        [Parameter(Mandatory=$true)][string[]]$ReachedStates,
        [Parameter(Mandatory=$true)][string[]]$RequestStates,
        [Parameter(Mandatory=$false)][int]$TimeoutSeconds=120
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $state = $null
    while ((Get-Date) -lt $deadline) {
        $state = get-procmgrstate -Name $Name
        if ($ReachedStates -contains $state) {
            return
        }
        if ($RequestStates -contains $state) {
            $result = invoke-procmgr -ProcmgrArgs @($Verb, $Name)
            if ($result.ExitCode -ne 0) {
                Write-Host -ForegroundColor Yellow "dd-procmgr $Verb $Name failed: $($result.Output -join ' ')"
            }
        }
        Start-Sleep -Seconds 1
    }
    $reported = if ($null -eq $state) { "unreadable" } else { $state }
    Write-Host -ForegroundColor Red "$Name is $reported after $TimeoutSeconds seconds, expected one of $($ReachedStates -join ', ')"
    exit 1
}

# A settled state is anything but Running that dd-procmgrd will not leave on its own: a process
# whose config gate is closed sits in Created and never transitions, and stopping runs both
# before and after the test body, so an already-stopped process is the normal case.
$settledStates = @("Stopped", "Created", "Exited", "Failed")

function stop-procmgrprocess {
    param([Parameter(Mandatory=$true)][string]$Name)
    converge-procmgrstate -Name $Name -Verb "stop" `
        -ReachedStates $settledStates -RequestStates @("Running")
}

function start-procmgrprocess {
    param([Parameter(Mandatory=$true)][string]$Name)
    converge-procmgrstate -Name $Name -Verb "start" `
        -ReachedStates @("Running") -RequestStates $settledStates
}

function stop-servicesfortest {
    # disable process agent so that it doesn't query the connections endpoint while we're running
    stop-procmgrprocess -Name datadog-agent-process

    # stop system probe to clean out any connections we're not interested in
    stop-procmgrprocess -Name datadog-agent-sysprobe
}

function make-connectionrequest {
    param(
        [Parameter(Mandatory=$true)] [string]$ConnExe,
        [Parameter(Mandatory=$true)] [string]$TargetHost,
        [Parameter(Mandatory=$false)] [string]$Port="80",
        [Parameter(Mandatory=$false)] [string]$TargetPath="/"
    )
    $list = @($TargetHost, $Port, $TargetPath)

    $targetpid = (start-process $ConnExe -passthru -NoNewWindow -ArgumentList $list).id    
    return $targetpid
}

function get-connectionsendpoint {
    $payload = (.\NamedPipeCmd.exe -method GET -path /network_tracer/connections -quiet) | convertfrom-json
    if (! $?){
        Write-Host -ForegroundColor Red "Failed to get connection list"
        exit 1
    }
    return $payload
}

stop-servicesfortest

## The test assumes some pretty specific setup. 

## assumptions
# That there is a datadog.json or app.config in the powershell directory 
#   C:\Windows\System32\WindowsPowerShell\v1.0
#   it sets  DD_SERVICE: powershell  DD_ENV: staging  DD_VERSION somever
# 
# That the IIS site has a datadog.json or web.config in the root
#   it sets the DD_SERVICE: service1  DD_ENV: staging DD_VERSION: 1.0-prerelease

## start the system probe
start-procmgrprocess -Name datadog-agent-sysprobe

## just give everything a chance to settle into place.
Start-Sleep -Seconds 5

## make a web request.  this will give us the PID we should look for in the connection list.
$targetpid = make-connectionrequest -ConnExe $ConnExe -TargetHost:$Targethost -Port:$Targetport -TargetPath:$TargetPath

start-sleep -seconds 1
## get the connection list.  This is going to have to change as soon as we go to named pipes.
$payload = get-connectionsendpoint

## for now, do it again until I sort out the first-time bug

## make a web request.  this will give us the PID we should look for in the connection list.
$targetpid = $targetpid = make-connectionrequest -ConnExe $ConnExe -TargetHost:$Targethost -Port:$Targetport -TargetPath:$TargetPath

# the etw works on a 3 second poll; make sure it's updated
start-sleep -seconds 6

## get the connection list.  This is going to have to change as soon as we go to named pipes.
$payload = get-connectionsendpoint

## stop the services for now.  this will clean up for next run
stop-servicesfortest

## taglist is an ordered list of tags, referenced in each conn object
$taglist = $payload.tags

## connections is the list of all the connections that were collected
$connections = $payload.conns

## find the client connection we're interested in
$client = $connections | where-object {$_.pid -eq $targetpid }

$clientport = $client.laddr.port

## find the server side of the connection
$server = $connections | where-object {$_.laddr.port -eq $Targetport -and $_.raddr.port -eq $clientport}

## for now, show the conns and tags

function validate-tags {
    param (
        [Parameter(Mandatory=$true)][array]$TagArray,
        [Parameter(Mandatory=$true)][array]$TagIndexes,
        [Parameter(Mandatory=$true)][array]$Expected
    )

    # walk the list of expected tags
    :expectedLoop foreach ($et in $Expected) {
        foreach ($idx in $TagIndexes) {
            if ($et -eq $TagArray[$idx]) {
                Write-Host -ForegroundColor Green "Matched tag $et"
                continue expectedLoop
            }
        }
        # if we get here, we didn't find the right tag
        Write-Host -ForegroundColor Red "Did not find expected tag $et"
        foreach ($found in $TagIndexes ) {
            Write-Host -ForegroundColor Yellow "Found tag $($TagArray[$found])"
        }
        return $false
    }
    return $true
}
if ($client.tags -eq $null) {
    $client.tags = @()
}
if ($server.tags -eq $null) {
    $server.tags = @()
}
$vt = validate-tags -TagArray $taglist -TagIndexes $client.tags -Expected $ExpectedClientTags
if ($vt -eq $false) {
    exit 1
}

$vt = validate-tags -TagArray $taglist -TagIndexes $server.tags -Expected $ExpectedServerTags
if ($vt -eq $false) {
    exit 1
}