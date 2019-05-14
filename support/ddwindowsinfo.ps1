param(
    [Parameter(Mandatory=$false)][switch]$uninstall = $false
)

$upgrade_code = "0c50421b-aefb-4f15-a809-7af256d608a5"
$product_name = "Datadog Agent"
$ddagentuser_name = "ddagentuser"
$dd_reg_root = "HKLM:\Software\Datadog\Datadog Agent"

$query = "Select Name,IdentifyingNumber,InstallDate,InstallLocation,ProductID,Version FROM Win32_Product where Name like '$product_name%'"
$userfilter = "name = '$ddagentuser_name'"


function getRegistryKeys{
    $ErrorActionPreference = "stop"
    Write-Host -ForegroundColor Green "Checking for Datadog Registry Entries"
    Try {
        $rroot = Get-ItemProperty -Path $dd_reg_root
    }
    Catch [System.Management.Automation.ItemNotFoundException] {
        Write-Host -ForegroundColor Yellow "Registry root not found"
    }

    if ($rroot) {
        Try {
            $croot = (Get-ItemProperty -Path $dd_reg_root -Name "ConfigRoot").ConfigRoot
        }
        Catch [System.Management.Automation.ItemNotFoundException] {
        }
        Catch [System.Management.Automation.PSArgumentException] {
        }
        Try {
            $ipath = (Get-ItemProperty -Path $dd_reg_root -Name "InstallPath").InstallPath
        }
        Catch [System.Management.Automation.ItemNotFoundException] {
        }
        Catch [System.Management.Automation.PSArgumentException] {
        }
        
    }
    return $croot, $ipath
}
Write-Host -ForegroundColor Green "Checking for Datadog Agent Installs (may take a while)..."
$installs = Get-WmiObject -query $query

if ($installs.Count -eq 0) {
    Write-Host -ForegroundColor Green "No installations of Datadog Agent found in install database"
} elseif ($installs.Count -gt 1) {
    Write-Host -ForegroundColor Yellow  "Found more than one installation of the Datadog Agent ?"
} else {
    Write-Host -ForegroundColor Green "Found 1 Datadog agent install"
}
foreach ($package in $installs) {
    Write-Host -ForegroundColor Green "Found installed $($package.Name)"
    Write-Host -ForegroundColor Green "                $($package.IdentifyingNumber)"
    Write-Host -ForegroundColor Green "                $($package.InstallDate)"
    Write-Host -ForegroundColor Green "                $($package.Version)"
}

Write-Host -ForegroundColor Green "Checking for ddagentuser"
$user = get-wmiobject win32_useraccount -Filter $userfilter
if ($user) {
    Write-Host -ForegroundColor Green "Found ddagentuser"
} else {
    Write-Host -ForegroundColor Green "Didn't find ddagentuser"
}

Write-host -ForegroundColor Green "Checking service installation"
$svc = sc.exe qc datadogagent
$svc_found = ($svc | Select-String "SUCCESS" -Quiet)

if ($svc_found) {
    Write-Host -ForegroundColor Green "Found datadog service installed"
} else {
    Write-Host -ForegroundColor Green "Didn't find datadog service"
}

$regpaths = getRegistryKeys
$configroot = $regpaths[0]
$installpath = $regpaths[1]

if(!$configroot){
    Write-Host -ForegroundColor Yellow "ConfigRoot property not found"
} else {
    Write-Host -ForegroundColor Green "ConfigRoot property $configroot"
}
if(!$installpath){
    Write-Host -ForegroundColor Yellow "InstallPath property not found"
} else {
    Write-Host -ForegroundColor Green "Install path property $installpath"
}


if(!$uninstall) {
    Write-Host -ForegroundColor Green "Agent check complete"
    return
}

Write-Host -ForegroundColor Green "`n=====================================================================================`n`n"
Write-Host -ForegroundColor Yellow "Attempting cleanup/uninstalls"
foreach ($package in $installs) {
    Write-Host -ForegroundColor Green  "Uninstalling existing agent"

    Write-Host -ForegroundColor Green "Found installed $($package.Name)"
    Write-Host -ForegroundColor Green "                $($package.IdentifyingNumber)"
    Write-Host -ForegroundColor Green "                $($package.InstallDate)"
    Write-Host -ForegroundColor Green "                $($package.Version)"

    $process = (Start-Process -FilePath msiexec -ArgumentList "/log dduninst.log /q /x $($package.IdentifyingNumber)" -Wait)
    if ($($process.ExitCode) -eq 0) {
        Write-Host -ForegroundColor Green "Uninstalled successfully"
    } else {
        Write-Host -ForegroundColor Yellow "Uninstall returned code $($process.ExitCode)"
    }
}

if($user) {
    $user_after_uninstall = get-wmiobject win32_useraccount -Filter $userfilter
    if ($user_after_uninstall) {
        Write-Host -ForegroundColor Yellow "Ddagentuser still present; removing"
        $netuser = & net user ddagentuser /DELETE
        Write-Host -ForegroundColor Green "User delete: $netuser"
    } else {
        Write-Host -ForegroundColor Green "ddagentuser deleted by uninstall"
    }
}

if($svc_found) {
    $svc = sc.exe qc datadogagent
    $svc_found_after_uninstall = ($svc | Select-String "SUCCESS" -Quiet)
    if($svc_found_after_uninstall) {
        Write-Host -ForegroundColor Yellow "Service still present after uninstall, removing"
        & net.exe /y stop datadogagent
        $scdelete = & sc.exe delete datadogagent
        Write-Host -ForegroundColor Green "Service delete code $scdelete"
    }
    else {
        Write-Host -ForegroundColor Green " service deleted by uninstall"
    }
}

$regpaths_after = getRegistryKeys
$configroot_after = $regpaths_after[0]
$installpath_after = $regpaths_after[1]

if($configroot) {
    if($configroot_after) {
        Write-Host -ForegroundColor Yellow "Deleting configroot key $configroot_after"
        Remove-ItemProperty -Path $dd_reg_root -Name ConfigRoot
    } else {
        Write-Host -ForegroundColor Green "Configroot removed by uninstall"
    }
}
if($installpath) {
    if($installpath_after) {
        Write-Host -ForegroundColor Yellow "Deleting InstallPath key $installpath_after"
        Remove-ItemProperty -Path $dd_reg_root -Name InstallPath
    } else {
        Write-Host -ForegroundColor Green "InstallPath removed by uninstall"
    }
}