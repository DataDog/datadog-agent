FROM mcr.microsoft.com/windows/servercore:1809

SHELL ["Powershell.exe", "-Command", "$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue';"]

RUN Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))

RUN Import-Module "$env:ChocolateyInstall\helpers\chocolateyInstaller.psm1" -Force;

RUN cinst -y git

RUN cinst -y 7zip

RUN cinst -y visualstudio2017buildtools

RUN cinst -y visualstudio2017-workload-vctools

RUN cinst -y vcpython27

RUN cinst -y wixtoolset --version 3.11
RUN [Environment]::SetEnvironmentVariable( \
    "Path", \
    [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";${env:ProgramFiles}\WiX Toolset v3.11\bin", \
    [System.EnvironmentVariableTarget]::Machine)

RUN cinst -y cmake
RUN [Environment]::SetEnvironmentVariable( \
    "Path", \
    [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";${env:ProgramFiles}\CMake\bin", \
    [System.EnvironmentVariableTarget]::Machine) \

RUN cinst -y golang --version 1.12.9

RUN cinst -y python2

RUN cinst -y ruby --version 2.4.3.1

RUN cinst -y msys2 --params "/NoUpdate" # install msys2 without system update

RUN Update-SessionEnvironment

RUN ridk install 2 3

RUN gem install bundler
