if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

Powershell -C "C:\mnt\tasks\winbuildscripts\Publish-Chocolatey-Package.ps1"
goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF