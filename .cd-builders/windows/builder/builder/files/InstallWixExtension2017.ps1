function Install-VsixExtension17
{
    Param
    (
        [String]$Url,
        [String]$Name
    )

    $ReleaseInPath = 'Community'
    $exitCode = -1

    try
    {
        Write-Host "Downloading $Name..."
        $FilePath = "${env:Temp}\$Name"

        Invoke-WebRequest -Uri $Url -OutFile $FilePath

        $ArgumentList = ('/quiet', $FilePath)

        Write-Host "Starting Install $Name..."
        $process = Start-Process -FilePath "C:\Program Files (x86)\Microsoft Visual Studio\2017\$ReleaseInPath\Common7\IDE\VSIXInstaller.exe" -ArgumentList $ArgumentList -Wait -PassThru
        $exitCode = $process.ExitCode

        if ($exitCode -eq 0 -or $exitCode -eq 3010)
        {
            Write-Host -Object 'Installation successful'
            return $exitCode
        }
        else
        {
            Write-Host -Object "Non zero exit code returned by the installation process : $exitCode."
            return $exitCode
        }
    }
    catch
    {
        Write-Host -Object "Failed to install the Extension $Name"
        Write-Host -Object $_.Exception.Message
        return -1
    }
}


choco install wixtoolset -y --force

#Installing VS extension 'Wix Toolset Visual Studio 2017 Extension'
$exitCode = Install-VsixExtension17 -Url 'https://github.com/wixtoolset/VisualStudioExtension/releases/download/v1.0.0.4/Votive2017.vsix' -Name 'Votive2017.vsix'
#return $exitCode
