::
:: Installs .NET Runtime 3.5, used by Wix 3.11 and the Visual C++ Compiler for Python 2.7
:: Taken from the mcr.microsoft.com/dotnet/framework/runtime:3.5 Dockerfile:
:: https://github.com/microsoft/dotnet-framework-docker/blob/abc2ca65b28058f7c71ec8cd8763a8fbf2a9c03f/3.5/runtime/windowsservercore-1909/Dockerfile
::

set DOTNET_RUNNING_IN_CONTAINER true

curl -fSLo microsoft-windows-netfx3.zip https://dotnetbinaries.blob.core.windows.net/dockerassets/microsoft-windows-netfx3-1909.zip
tar -zxf microsoft-windows-netfx3.zip
del /F /Q microsoft-windows-netfx3.zip
DISM /Online /Quiet /Add-Package /PackagePath:.\microsoft-windows-netfx3-ondemand-package~31bf3856ad364e35~amd64~~.cab
del microsoft-windows-netfx3-ondemand-package~31bf3856ad364e35~amd64~~.cab
powershell Remove-Item -Force -Recurse ${Env:TEMP}\*

curl -fSLo patch.msu http://download.windowsupdate.com/d/msdownload/update/software/updt/2020/01/windows10.0-kb4534132-x64-ndp48_21067bd5f9c305ee6a6cee79db6ca38587cb6ad8.msu
mkdir patch
expand patch.msu patch -F:*
del /F /Q patch.msu
DISM /Online /Quiet /Add-Package /PackagePath:C:\patch\windows10.0-kb4534132-x64-ndp48.cab
rmdir /S /Q patch