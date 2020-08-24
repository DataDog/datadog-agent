::
:: Installs .NET Runtime 3.5, used by Wix 3.11 and the Visual C++ Compiler for Python 2.7
:: Taken from the mcr.microsoft.com/dotnet/framework/runtime:3.5 Dockerfile:
:: https://github.com/microsoft/dotnet-framework-docker/blob/26597e42d157cc1e09d1e0dc8f23c32e6c3d1467/3.5/runtime/windowsservercore-ltsc2019/Dockerfile
::

curl -fSLo microsoft-windows-netfx3.zip https://dotnetbinaries.blob.core.windows.net/dockerassets/microsoft-windows-netfx3-1809.zip
tar -zxf microsoft-windows-netfx3.zip
del /F /Q microsoft-windows-netfx3.zip
DISM /Online /Quiet /Add-Package /PackagePath:.\microsoft-windows-netfx3-ondemand-package~31bf3856ad364e35~amd64~~.cab
del microsoft-windows-netfx3-ondemand-package~31bf3856ad364e35~amd64~~.cab
powershell Remove-Item -Force -Recurse ${Env:TEMP}\*

curl -fSLo patch.msu http://download.windowsupdate.com/c/msdownload/update/software/updt/2020/01/windows10.0-kb4534119-x64_a2dce2c83c58ea57145e9069f403d4a5d4f98713.msu
mkdir patch
expand patch.msu patch -F:*
del /F /Q patch.msu
DISM /Online /Quiet /Add-Package /PackagePath:C:\patch\windows10.0-kb4534119-x64.cab
rmdir /S /Q patch