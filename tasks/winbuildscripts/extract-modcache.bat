if exist c:\mnt\modcache.tar (
    @echo Extracting modcache
    Powershell -C "7z x c:\mnt\modcache.tar -oc:\\"
    del /f c:\mnt\modcache.tar
    @echo Modchache extracted
) else (
    @echo modcache.tar not found, dependencies will be downloaded
)
