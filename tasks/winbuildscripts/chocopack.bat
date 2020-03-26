if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

Powershell -C "c:\mnt\tasks\winbuildscripts\Generate-Chocolatey-Package.ps1"
