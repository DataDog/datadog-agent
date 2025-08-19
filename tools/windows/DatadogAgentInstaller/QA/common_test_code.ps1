# This code is common to all tests
# It will be executed in the context of the guest machine

class InstallResult {
    [int]$StatusCode
    [string[]]$InstallLogs

    InstallResult([int]$statusCode, [string]$installLogs) {
        $this.StatusCode = $statusCode;
        # Need to split into multiple lines otherwise select-string
        # will just return the entire logs if there's any match
        $this.InstallLogs = $installLogs.Split("`n");
    }
}

function Get-TimeStamp {
    return "[{0:MM/dd/yy} {0:HH:mm:ss}]" -f (Get-Date)
}

function report_info($message) {
    Write-Host "$(Get-TimeStamp) == $message"
}

function report_warn($message) {
    Write-Warning "$(Get-TimeStamp) ⚠ $message"
}

function report_err($message) {
    Write-Error "$(Get-TimeStamp) ❌ $message"
}

function report_success($message) {
    Write-Host "$(Get-TimeStamp) ✅ $message"
}

function Wait-For-Msiexec-Lock($action = $null) {
    $val = 0
    $retryCount = 30
    # Retry 30 times at 10s intervals => 5min.
    while ($val -ne $retryCount)
    {
        try
        {
            if ($val -gt 0) {
                report_info("Checking for other installation, retry number $val")
            }
            $Mutex = [System.Threading.Mutex]::OpenExisting("Global\_MSIExecute");
            $Mutex.Dispose();
            # Another installer is running
            Try {
                $msiInProgressCmdLine =
                    Get-WmiObject -Class 'Win32_Process' -Filter "name = 'msiexec.exe'" -ErrorAction 'Stop' |
                    Where-Object { $_.CommandLine } |
                    Select-Object -ExpandProperty 'CommandLine' |
                    Where-Object { $_ -match '/x' -or $_ -match '/i' } |
                    ForEach-Object { $_.Trim() }
            }
            Catch { }
            report_warn("The following MSI installation is in progress [$msiInProgressCmdLine] waiting 10s.")
            $val++
            Start-Sleep -s 10
        }
        catch
        {
            # Lock is free
            if ($action) {
                $result = $action.Invoke()
                if ($result.ExitCode -ne 0) {
                    return $result.ExitCode
                }
            }
            return 0
        }
    }

    if ($val -eq $retryCount) {
        return -1
    }
}

function Install-MSI() {
    param (
        [Parameter()][string]$installerName,
        [Parameter()][string]$msiArgs
    )
    report_info("Installing $installerName with args $msiArgs")
    $logPath = [System.IO.Path]::GetTempFileName()
    $result = Wait-For-Msiexec-Lock { return Start-Process "msiexec" -ArgumentList "/qn /i C:\$installerName /log $logPath $msiArgs"  -NoNewWindow -Wait -Passthru }
    return [InstallResult]::new($result, (Get-Content $logPath -ErrorAction SilentlyContinue | Out-String))
}

function Uninstall-MSI() {
    param (
        [Parameter()][string]$installerName,
        [Parameter()][string]$msiArgs
    )
    report_info("Uninstalling $installerName with args $msiArgs")
    $logPath = [System.IO.Path]::GetTempFileName()
    $result = Wait-For-Msiexec-Lock { return Start-Process "msiexec" -ArgumentList "/qn /x C:\$installerName /log $logPath $msiArgs"  -NoNewWindow -Wait -Passthru }
    return [InstallResult]::new($result, (Get-Content $logPath -ErrorAction SilentlyContinue | Out-String))
}

function Uninstall-DatadogAgent() {
    report_info("Uninstalling Datadog Agent")
    $installed =  Get-WmiObject win32_product -filter "Name LIKE '%datadog%'"
    if ($installed -eq $null)
    {
        report_info("Didn't find any installations of Datadog Agent")
        return
    }
    $logPath = [System.IO.Path]::GetTempFileName()
    $result = Wait-For-Msiexec-Lock { return Start-Process "msiexec" -ArgumentList "/qn /x $($installed.IdentifyingNumber) /log $logPath"  -NoNewWindow -Wait -Passthru }
    return [InstallResult]::new($result, (Get-Content $logPath -ErrorAction SilentlyContinue | Out-String))
}

function Is_DaclAutoInherited_Flag_Set([string]$path) {
    $controlFlags = (New-Object System.Security.AccessControl.RawSecurityDescriptor @((Get-Acl -Path $path).GetSecurityDescriptorBinaryForm(), 0)).ControlFlags
    return $controlFlags.HasFlag([System.Security.AccessControl.ControlFlags]::DiscretionaryAclAutoInherited)
}

function Assert_DaclAutoInherited_Flag_Set([string]$path) {
    Assert_DirectoryExists($path)
    if ((Is_DaclAutoInherited_Flag_Set($path)) -eq $false) {
        report_err "Inheritance flag is not set on $($path) when it should be"
        Exit -1
    }
}

function Assert_DaclAutoInherited_Flag_NotSet([string]$path) {
    Assert_DirectoryExists($path)
    if ((Is_DaclAutoInherited_Flag_Set($path)) -eq $true) {
        report_err "Inheritance flag is set on $($path) when it should not be"
        Exit -1
    }
}

function Assert_DirectoryExists([string]$path) {
    if (-Not (Test-Path -Path $path)) {
        report_err "Path $($path) doesn't exists when it should"
        Exit -1
    }
}

function Assert_DirectoryNotExists([string]$path) {
    if (Test-Path -Path $path) {
        report_err "Path $($path) exists when it should not"
        Exit -1
    }
}

function Check_DaclAutoInherited_Flag([string]$path) {
    report_info ("{0} SE_DACL_AUTOINHERITED_SET: {1}" -f $path, (Is_DaclAutoInherited_Flag_Set -Path $path))
}

function Assert_InstallSuccess([InstallResult]$result) {
    if ($result.StatusCode -eq -1) {
        report_err("Expected installation to succeed, but it failed because we could not acquire the MSI lock")
        Exit -1
    } elseif ($result.StatusCode -ne 0) {
        report_err("Expected installation to succeed, but it failed with code {0}" -f $result.StatusCode)
        $result.InstallLogs | Write-Host
        Exit -1
    }
}

function Assert_InstallFinishedWithCode([InstallResult]$result, [int[]]$expectedCodes) {
    if ($result.StatusCode -notin $expectedCodes) {
        report_err("Expected installation to finish with a code in [{1}], but was {0}" -f $result.StatusCode, ($expectedCodes -join ','))
        Exit -1
    }
}
