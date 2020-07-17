if ($ENV:PACKER_BUILDER_TYPE -eq "hyperv-iso") {
  Write-Output "Nothing to do for Hyper-V"
  exit 0
}

$isopath = "C:\Windows\Temp\windows.iso"

# Mount the .iso, then build the path to the installer by getting the Driveletter attribute from Get-DiskImage piped into Get-Volume and adding a :\setup.exe
# A separate variable is used for the parameters. There are cleaner ways of doing this. I chose the /qr MSI Installer flag because I personally hate silent installers
# Even though our build is headless. 

Write-Output "Mounting disk image at $isopath"
Mount-DiskImage -ImagePath $isopath

function parallels {
    $exe = ((Get-DiskImage -ImagePath $isopath | Get-Volume).Driveletter + ':\PTAgent.exe')
    $parameters = '/install_silent'

    $process = Start-Process $exe $parameters -Wait -PassThru
    if ($process.ExitCode -eq 0) {
    Write-Host "Installation Successful"
    } elseif ($process.ExitCode -eq 3010) {
    Write-Warning "Installation Successful, Please reboot"
    } else {
    Write-Error "Installation Failed: Error $($process.ExitCode)"
    Start-Sleep 2
    exit $process.ExitCode
    }
}

function vmware {
    $exe = ((Get-DiskImage -ImagePath $isopath | Get-Volume).Driveletter + ':\setup.exe')
    $parameters = '/S /v "/qr REBOOT=R"'

    Start-Process $exe $parameters -Wait
}

function virtualbox {
    $certdir = ((Get-DiskImage -ImagePath $isopath | Get-Volume).Driveletter + ':\cert\')
    $VBoxCertUtil = ($certdir + 'VBoxCertUtil.exe')

    # Added support for VirtualBox 4.4 and above by doing this silly little trick.
    # We look for the presence of VBoxCertUtil.exe and use that as the deciding factor for what method to use.
    # The better way to do this would be to parse the Virtualbox version file that Packer can upload, but this was quick.

    if (Test-Path ($VBoxCertUtil)) {
            Write-Output "Using newer (4.4 and above) certificate import method"
        Get-ChildItem $certdir *.cer | ForEach-Object { & $VBoxCertUtil add-trusted-publisher $_.FullName --root $_.FullName}
    }

    else {
            Write-Output "Using older (4.3 and below) certificate import method"
        $certpath = ($certpath + 'oracle-vbox.cer')
        certutil -addstore -f "TrustedPublisher" $certpath
    }

    $exe = ((Get-DiskImage -ImagePath $isopath | Get-Volume).Driveletter + ':\VBoxWindowsAdditions.exe')
    $parameters = '/S'

    Start-Process $exe $parameters -Wait
}

if ($ENV:PACKER_BUILDER_TYPE -eq "vmware-iso") {
    Write-Output "Installing VMWare Guest Tools"
    vmware
} elseif ($env:PACKER_BUILDER_TYPE -match 'parallels') {
    Write-Output "Installing Parallels Guest Tools"
    parallels
} else {
    Write-Output "Installing Virtualbox Guest Tools"
    virtualbox
}

#Time to clean up - dismount the image and delete the original ISO

Write-Output "Dismounting disk image $isopath"
Dismount-DiskImage -ImagePath $isopath
Write-Output "Deleting $isopath"
Remove-Item $isopath