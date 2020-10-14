# Use the Microsoft-provided .NET Runtime 4.8 image as the base image
# because installing it in the image with Chocolatey requires a reboot.

# There is a bug in the February 11, 2020 update that makes commands fail in
# docker containers (see: https://support.microsoft.com/en-us/help/4542617/you-might-encounter-issues-when-using-windows-server-containers-with-t)
# To avoid that, there are two solutions:
# - both the host and the container must have the February 11, 2020 update, or
# - neither the host and the container must have the February 11, 2020 update.
# Since our 1809 windows-docker host image does not have this update, we use a base
# container image that does not have this update either (thus the 20200114 tag).
# On the contrary, since our 1909 windows-docker host image does have this update,
# we use a base container image that does have this update.
# TODO: Once the 1809 windows-docker host image is updated, update the base container image.
ARG BASE_IMAGE=mcr.microsoft.com/dotnet/framework/runtime:4.8-20200114-windowsservercore-ltsc2019

FROM ${BASE_IMAGE}

SHELL ["powershell", "-Command"]

ARG WINDOWS_VERSION
ENV WINDOWS_VERSION=${WINDOWS_VERSION:-1809}

ARG DD_TARGET_ARCH
ENV TARGET_ARCH=${DD_TARGET_ARCH:-x64}

# Chocolatey package versions
ENV GIT_VERSION "2.26.2"
ENV SEVENZIP_VERSION "19.0"
ENV VS2017BUILDTOOLS_VERSION "15.9.23.0"
ENV VCPYTHON27_VERSION "9.0.0.30729"
ENV GO_VERSION "1.13.11"
ENV RUBY_VERSION "2.4.3.1"
ENV PYTHON_VERSION "3.8.2"
ENV WIX_VERSION "3.11.2"
ENV CMAKE_VERSION "3.17.2"
ENV MSYS_VERSION "20190524.0.0.20191030"

ENV EMBEDDED_PYTHON_2_VERSION "2.7.17"
ENV EMBEDDED_PYTHON_3_VERSION "3.8.1"

ENV CACERTS_HASH "bb28d145ed1a4ee67253d8ddb11268069c9dafe3db25a9eee654974c4e43eee5"

LABEL target_agent="Agent 6/7"
LABEL target_arch=${TARGET_ARCH}
LABEL windows_version=${WINDOWS_VERSION}
LABEL git_version=${GIT_VERSION}
LABEL sevenzip_version=${SEVENZIP_VERSION}
LABEL vs2017buildtools_version=${VS2017BUILDTOOLS_VERSION}
LABEL vcpython27_version=${VCPYTHON27_VERSION}
LABEL go_version=${GO_VERSION}
LABEL ruby_version=${RUBY_VERSION}
LABEL wix_version=${WIX_VERSION}
LABEL cmake_version=${CMAKE_VERSION}
LABEL msys_version=${MSYS_VERSION}
LABEL system_python_version=${PYTHON_VERSION}
LABEL embedded_py2_version=${EMBEDDED_PYTHON_2_VERSION}
LABEL embedded_py3_version=${EMBEDDED_PYTHON_3_VERSION}

# Add certificates needed for build & check certificates file hash
# We need to trust the DigiCert High Assurance EV Root CA certificate, which signs python.org,
# to be able to download some Python components during the Agent build.
RUN (New-Object System.Net.WebClient).DownloadFile(\"https://curl.haxx.se/ca/cacert.pem\", \"cacert.pem\")
RUN if ((Get-FileHash .\cacert.pem).Hash -ne \"$ENV:CACERTS_HASH\") { Write-Host \"Wrong hashsum for cacert.pem: got '$((Get-FileHash .\cacert.pem).Hash)', expected '$ENV:CACERTS_HASH'.\"; exit 1 }
RUN setx SSL_CERT_FILE \"C:\cacert.pem\"

### Preliminary step: we need both the .NET 3.5 runtime and
### the .NET 4.8 runtime. To do this, we get 4.8 from a base image and we
### manually the install .NET Framework 3.5 runtime using the instructions in
### the mcr.microsoft.com/dotnet/framework/runtime:3.5 Dockerfile:
### https://github.com/microsoft/dotnet-framework-docker/blob/26597e42d157cc1e09d1e0dc8f23c32e6c3d1467/3.5/runtime/windowsservercore-ltsc2019/Dockerfile

### The .NET Fx 3.5 is needed for the Visual C++ Compiler for Python 2.7
### (https://www.microsoft.com/en-us/download/details.aspx?id=44266)
### and to work around a bug in the WiX 3.11 installer
### (https://github.com/wixtoolset/issues/issues/5661).

# Install .NET Fx 3.5
COPY install_net35_1809.bat install_net35_1809.bat
COPY install_net35_1909.bat install_net35_1909.bat
RUN if ($Env:WINDOWS_VERSION -eq '1809') { .\install_net35_1809.bat }
RUN if ($Env:WINDOWS_VERSION -eq '1909') { .\install_net35_1909.bat }

### End of preliminary step

RUN if ($Env:TARGET_ARCH -eq 'x86') { setx CHOCO_ARCH_FLAG '-x86' }

# Install Chocolatey
RUN $env:chocolateyUseWindowsCompression = 'true'; Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))

# Install git
RUN $env:chocolateyUseWindowsCompression = 'true'; cinst -y --no-progress git $ENV:CHOCO_ARCH_FLAG --version $ENV:GIT_VERSION
### HACK: we disable symbolic links when cloning repositories
### to work around a symlink-related failure in the agent-binaries omnibus project
### when copying the datadog-agent project twice.
RUN git config --system core.symlinks false

# Install 7zip
RUN $env:chocolateyUseWindowsCompression = 'true'; cinst -y --no-progress 7zip $ENV:CHOCO_ARCH_FLAG --version $ENV:SEVENZIP_VERSION

# Install VS2017
RUN cinst -y --no-progress visualstudio2017buildtools $ENV:CHOCO_ARCH_FLAG --version $ENV:VS2017BUILDTOOLS_VERSION --params \"--add Microsoft.VisualStudio.ComponentGroup.NativeDesktop.Win81 --add Microsoft.VisualStudio.Workload.VCTools --add Microsoft.VisualStudio.Component.VC.Runtimes.x86.x64.Spectre --add Microsoft.VisualStudio.Component.Windows10SDK.17763\"
RUN setx VSTUDIO_ROOT \"${env:ProgramFiles(x86)}\Microsoft Visual Studio\2017\BuildTools\"

# If x64, install the WDK for driver development
COPY install_wdk.ps1 install_wdk.ps1
RUN if ($Env:TARGET_ARCH -eq 'x64') { powershell -Command .\install_wdk.ps1 }

# Install VC compiler for Python 2.7
RUN cinst -y --no-progress vcpython27 $ENV:CHOCO_ARCH_FLAG --version $ENV:VCPYTHON27_VERSION

# Install Wix and update PATH to include it
RUN cinst -y --no-progress wixtoolset $ENV:CHOCO_ARCH_FLAG --version $ENV:WIX_VERSION
RUN [Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\", [EnvironmentVariableTarget]::Machine) + \";${env:ProgramFiles(x86)}\WiX Toolset v3.11\bin\", [System.EnvironmentVariableTarget]::Machine)

# Install Cmake and update PATH to include it
RUN cinst -y --no-progress cmake $ENV:CHOCO_ARCH_FLAG --version $ENV:CMAKE_VERSION
RUN if ($Env:TARGET_ARCH -eq 'x86') { [Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\", [EnvironmentVariableTarget]::Machine) + \";${Env:ProgramFiles(x86)}\CMake\bin\", [System.EnvironmentVariableTarget]::Machine) }
RUN if ($Env:TARGET_ARCH -eq 'x64') { [Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\", [EnvironmentVariableTarget]::Machine) + \";${env:ProgramFiles}\CMake\bin\", [System.EnvironmentVariableTarget]::Machine) }

# Install golang and set GOPATH to the dev path used in builds & tests
RUN cinst -y --no-progress golang $ENV:CHOCO_ARCH_FLAG --version $ENV:GO_VERSION
RUN setx GOPATH C:\dev\go

# Install system Python 3 (to use invoke).
# We always install the 64 bit version because vcredist140 won't work otherwise
RUN cinst -y --no-progress python3 --version $ENV:PYTHON_VERSION

# Install 64-bit ruby and bundler (for omnibus builds)
RUN cinst -y --no-progress ruby --version $ENV:RUBY_VERSION
RUN setx RIDK ((Get-Command ridk).Path)
RUN gem install bundler

# Install msys2 system & install 64-bit C/C++ compilation toolchain
RUN cinst -y --no-progress msys2 --params \"/NoUpdate\" --version $ENV:MSYS_VERSION
RUN ridk install 3

# (32-bit only) Install 32-bit C/C++ compilation toolchain
RUN if ($Env:TARGET_ARCH -eq 'x86') { ridk enable; bash -c \"pacman -S --needed --noconfirm mingw-w64-i686-binutils mingw-w64-i686-crt-git mingw-w64-i686-gcc mingw-w64-i686-gcc-libs mingw-w64-i686-headers-git mingw-w64-i686-libmangle-git mingw-w64-i686-libwinpthread-git mingw-w64-i686-make mingw-w64-i686-pkg-config mingw-w64-i686-tools-git mingw-w64-i686-winpthreads-git\" }
RUN if ($Env:TARGET_ARCH -eq 'x86') { [Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\", [EnvironmentVariableTarget]::Machine) + \";C:\tools\msys64\mingw32\bin;C:\tools\msys64\usr\bin\", [System.EnvironmentVariableTarget]::Machine) }

# Install aws cli
COPY install_awscli.ps1 install_awscli.ps1
RUN powershell -C .\install_awscli.ps1

# Install docker, manifest-tool and notary
COPY install_docker.ps1 install_docker.ps1
RUN powershell -Command .\install_docker.ps1

# Install embedded pythons (for unit testing)
COPY install_embedded_pythons.ps1 install_embedded_pythons.ps1
RUN powershell -C .\install_embedded_pythons.ps1

# Add signtool to path
RUN [Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\", [EnvironmentVariableTarget]::Machine) + \";${env:ProgramFiles(x86)}\Windows Kits\8.1\bin\x64\", [System.EnvironmentVariableTarget]::Machine)

# Set 32-bit flag env var
RUN if ($Env:TARGET_ARCH -eq 'x86') { setx WINDOWS_BUILD_32_BIT 1 }

COPY entrypoint.bat /entrypoint.bat
COPY aws_networking.ps1 /aws_networking.ps1

ENTRYPOINT ["/entrypoint.bat"]
