# Installation ways:


## Script parameters:

```
    [Parameter(Mandatory = $true)]
    [string]$stsApiKey = "API_KEY",
    [string]$stsUrl = "https://test-stackstate-agent.sts/stsAgent",
    [string]$stsHostname = $env:computername,
    [string]$stsSkipSSLValidation = "true",
    [string]$stsCodeName = "master",
    [string]$stsAgentVersion = "2.0.0.git.443.ef0c11ef"

```


## Manual

```ps

# optional download
(new-object net.webclient).DownloadFile('https://location/on/a/web/for/agent-win.ps1','c:\agent-win.ps1')
# install with optional overrides
.\agent-win.ps1 -stsApiKey AAA -stsUrl BBB -stsHostname CCC -stsSkipSSLValidation false -stsAgentVersion DDD

```

## X-Liner from pre-downloaded script

Put overrides only into $stsAgentParams , other will be picked up from default values in install script


```ps

$stsAgentParams = @{
    stsApiKey = 'AAAA'
    stsUrl='BBB'
    stsHostname='CCC'
    stsSkipSSLValidation='true'
    stsCodeName='DDD'
    stsAgentVersion='EEE'
}

$ScriptPath = 'c:\agent-win.ps1'
$sb = [scriptblock]::create(".{$(get-content $ScriptPath -Raw)} $(&{$args} @stsAgentParams)")
Invoke-Command -ScriptBlock $sb

```



## X-Liner

Put overrides only into $stsAgentParams , other will be picked up from default values in install script

```ps

$stsAgentParams = @{
    stsApiKey = 'AAAA'
    stsUrl='BBB'
    stsHostname='CCC'
    stsSkipSSLValidation='true'
    stsCodeName='DDD'
    stsAgentVersion='EEE'
}



$ScriptPath = ((new-object net.webclient).DownloadString('https://location/on/a/web/for/agent-win.ps1'))
$sb = [scriptblock]::create(".{$(ScriptPath)} $(&{$args} @stsAgentParams)")
Invoke-Command -ScriptBlock $sb

```

# One liner using module

Requiries installation logic packed as a powershell module (see `agent-win-module.ps1`)

```ps

. { iwr -useb https://location/on/a/web/for/agent-win-module.ps1 } | iex; install -stsApiKey AAA -stsUrl BBB -stsHostname CCC -stsSkipSSLValidation false -stsAgentVersion DD

```
