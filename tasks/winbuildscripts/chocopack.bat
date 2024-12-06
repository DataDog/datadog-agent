if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

Powershell -C "C:\mnt\tasks\winbuildscripts\Generate-Chocolatey-Package.ps1 %1 %2" || exit /b 1
goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 2
goto :EOF
