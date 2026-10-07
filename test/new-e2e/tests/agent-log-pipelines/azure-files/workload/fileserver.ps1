<#
.SYNOPSIS
Runs the Windows file server of the Azure Files E2E's smb-windows cell.

.DESCRIPTION
The test copies this script, logwriter.py, sidecars.py and the scheduled task
wrappers (<Root>\run\<task>.cmd) to the VM over SFTP, then runs one action of
this script over SSH (see windows.go and windows_test.go):

- prepare: stops the previous run's writer and removes its share, requires SMB
  signing on the server, opens TCP 445, creates or resets the local user that
  the Agent reads the share as, with the password in PasswordFile (the file is
  deleted at once and the password is never printed), creates the share with
  read access for that user alone, installs the embeddable Python once, and
  registers the writer, the ledger and the appender as scheduled tasks;
- start: starts those tasks;
- read: prints every file of Paths that exists, one after the other, sharing
  them with the processes still writing them, and fails when none exists;
- sessions: prints, as JSON, whether the server requires signing and the SMB
  sessions of the user;
- describe: prints the server's state for the evidence.
#>
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet('prepare', 'start', 'read', 'sessions', 'describe')]
    [string] $Action,
    [string] $Root = 'C:\azure-files-e2e',
    [string] $ShareName,
    [string] $UserName,
    [string] $PasswordFile,
    [string] $PythonUrl,
    [string] $PythonDir,
    [string[]] $Paths,
    [string] $TaskPrefix = 'azure-files-e2e'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0
$ProgressPreference = 'SilentlyContinue'

$tasks = @('writer', 'ledger', 'appender')
$sharesDir = Join-Path $Root 'shares'

function Stop-Workload {
    foreach ($task in $tasks) {
        $name = "$TaskPrefix-$task"
        if (Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue) {
            Stop-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
            Unregister-ScheduledTask -TaskName $name -Confirm:$false
        }
    }
    # Stopping a task ends its cmd.exe; the python.exe it started may outlive it.
    $pattern = [regex]::Escape($Root)
    foreach ($process in @(Get-CimInstance -ClassName Win32_Process -Filter "Name = 'python.exe'")) {
        if ($process.CommandLine -and $process.CommandLine -match $pattern) {
            Invoke-CimMethod -InputObject $process -MethodName Terminate | Out-Null
        }
    }
}

function Remove-PreviousShares {
    foreach ($share in @(Get-SmbShare | Where-Object { $_.Path -like "$sharesDir\*" })) {
        Remove-SmbShare -Name $share.Name -Force
    }
    if (Test-Path -LiteralPath $sharesDir) {
        # A file still held open is left behind rather than failing the run.
        Remove-Item -LiteralPath $sharesDir -Recurse -Force -ErrorAction Continue
    }
}

function Set-ReaderAccount {
    try {
        $plain = [System.IO.File]::ReadAllText($PasswordFile).TrimEnd([char[]]"`r`n")
        $password = ConvertTo-SecureString -String $plain -AsPlainText -Force
        $plain = $null
    } finally {
        Remove-Item -LiteralPath $PasswordFile -Force -ErrorAction SilentlyContinue
    }
    if (Get-LocalUser -Name $UserName -ErrorAction SilentlyContinue) {
        Set-LocalUser -Name $UserName -Password $password -PasswordNeverExpires $true
        Enable-LocalUser -Name $UserName
    } else {
        New-LocalUser -Name $UserName -Password $password -PasswordNeverExpires -UserMayNotChangePassword `
            -Description 'Azure Files E2E smb-windows reader' | Out-Null
    }
}

function New-ReaderShare {
    $dir = Join-Path $sharesDir $ShareName
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    # The writer runs as SYSTEM and keeps the rights the directory inherits;
    # the reader may only read what it writes.
    $account = "$env:COMPUTERNAME\$UserName"
    $acl = Get-Acl -LiteralPath $dir
    $rule = New-Object -TypeName System.Security.AccessControl.FileSystemAccessRule `
        -ArgumentList @($account, 'ReadAndExecute', 'ContainerInherit, ObjectInherit', 'None', 'Allow')
    $acl.AddAccessRule($rule)
    Set-Acl -LiteralPath $dir -AclObject $acl
    New-SmbShare -Name $ShareName -Path $dir -ReadAccess $account | Out-Null
}

function Install-Python {
    $python = Join-Path $PythonDir 'python.exe'
    if (-not (Test-Path -LiteralPath $python)) {
        $zip = Join-Path $Root 'python-embed.zip'
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -UseBasicParsing -Uri $PythonUrl -OutFile $zip
        Expand-Archive -LiteralPath $zip -DestinationPath $PythonDir -Force
        Remove-Item -LiteralPath $zip -Force
    }
    # python.org signs its Windows binaries; anything else is not run.
    $signature = Get-AuthenticodeSignature -FilePath $python
    $subject = ''
    if ($null -ne $signature.SignerCertificate) {
        $subject = $signature.SignerCertificate.Subject
    }
    if ($signature.Status -ne 'Valid' -or $subject -notmatch 'O=Python Software Foundation') {
        Remove-Item -LiteralPath $PythonDir -Recurse -Force -ErrorAction SilentlyContinue
        throw "python.exe from $PythonUrl is not signed by the Python Software Foundation: status $($signature.Status), signer '$subject'"
    }
    $version = & $python -c 'import sys; print(sys.version.split()[0])'
    if ($LASTEXITCODE -ne 0) {
        throw "$python does not run"
    }
    Write-Output "python_ready path=$python version=$version"
}

function Register-Workload {
    foreach ($task in $tasks) {
        $wrapper = Join-Path $Root "run\$task.cmd"
        if (-not (Test-Path -LiteralPath $wrapper)) {
            throw "the test did not copy $wrapper"
        }
        $action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\cmd.exe" -Argument "/d /c `"$wrapper`""
        $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
        # No time limit: the writer runs until the next run's prepare stops it.
        $settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew `
            -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
        Register-ScheduledTask -TaskName "$TaskPrefix-$task" -Action $action -Principal $principal -Settings $settings -Force | Out-Null
    }
}

function Read-SharedFile([string] $path) {
    $stream = [System.IO.File]::Open($path, 'Open', 'Read', 'ReadWrite, Delete')
    try {
        $reader = [System.IO.StreamReader]::new($stream)
        return $reader.ReadToEnd()
    } finally {
        $stream.Dispose()
    }
}

switch ($Action) {
    'prepare' {
        Stop-Workload
        Remove-PreviousShares
        # Every session the server accepts must be signed. The Agent signs or
        # encrypts every session it opens.
        Set-SmbServerConfiguration -RequireSecuritySignature $true -EnableSecuritySignature $true -Force
        $rule = "$TaskPrefix-smb-in"
        if (-not (Get-NetFirewallRule -Name $rule -ErrorAction SilentlyContinue)) {
            New-NetFirewallRule -Name $rule -DisplayName $rule -Direction Inbound -Protocol TCP -LocalPort 445 `
                -Action Allow -Profile Any | Out-Null
        }
        Set-ReaderAccount
        New-ReaderShare
        New-Item -ItemType Directory -Path (Join-Path $Root 'logs') -Force | Out-Null
        Install-Python
        Register-Workload
        Write-Output "fileserver_prepared share=$ShareName user=$UserName"
    }
    'start' {
        foreach ($task in $tasks) {
            Start-ScheduledTask -TaskName "$TaskPrefix-$task"
        }
        Start-Sleep -Seconds 5
        foreach ($task in $tasks) {
            $state = (Get-ScheduledTask -TaskName "$TaskPrefix-$task").State
            if ("$state" -ne 'Running') {
                # The task's console log says why its script stopped.
                $log = Join-Path $Root "logs\$ShareName-$task.log"
                $tail = 'no log'
                if (Test-Path -LiteralPath $log) {
                    $tail = (Get-Content -LiteralPath $log -Tail 20) -join "`n"
                }
                throw "scheduled task $TaskPrefix-$task is $state; the end of $log`n$tail"
            }
        }
        Write-Output "fileserver_started tasks=$($tasks -join ',')"
    }
    'read' {
        $found = $false
        foreach ($path in $Paths) {
            if (Test-Path -LiteralPath $path -PathType Leaf) {
                [Console]::Out.Write((Read-SharedFile $path))
                $found = $true
            }
        }
        if (-not $found) {
            throw "none of $($Paths -join ', ') exists"
        }
    }
    'sessions' {
        $config = Get-SmbServerConfiguration
        $sessions = @(Get-SmbSession | Where-Object { $_.ClientUserName -like "*\$UserName" } |
                Select-Object ClientComputerName, ClientUserName, Dialect, NumOpens, SecondsExists)
        $result = [ordered]@{
            require_security_signature = [bool]$config.RequireSecuritySignature
            encrypt_data               = [bool]$config.EncryptData
            sessions                   = $sessions
        }
        [Console]::Out.Write((ConvertTo-Json -InputObject $result -Depth 4 -Compress))
    }
    'describe' {
        $sections = [ordered]@{
            'operating system' = { Get-CimInstance -ClassName Win32_OperatingSystem | Format-List Caption, Version, BuildNumber }
            'smb server'       = { Get-SmbServerConfiguration | Format-List RequireSecuritySignature, EnableSecuritySignature, EncryptData, RejectUnencryptedAccess, EnableSMB1Protocol, EnableSMB2Protocol }
            'share'            = { Get-SmbShare -Name $ShareName | Format-List * }
            'share access'     = { Get-SmbShareAccess -Name $ShareName | Format-Table -AutoSize }
            'sessions'         = { Get-SmbSession | Format-List * }
            'open files'       = { Get-SmbOpenFile | Format-List * }
            'tasks'            = { Get-ScheduledTask -TaskName "$TaskPrefix-*" | Format-Table TaskName, State -AutoSize }
            'files'            = { Get-ChildItem -LiteralPath (Join-Path $sharesDir $ShareName) | Format-Table Name, Length, LastWriteTimeUtc -AutoSize }
        }
        foreach ($name in $sections.Keys) {
            Write-Output "==== $name ===="
            try {
                & $sections[$name] | Out-String -Width 250
            } catch {
                Write-Output "failed: $_"
            }
        }
    }
}
