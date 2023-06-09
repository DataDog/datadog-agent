if exist c:\mnt\modcache.tar.zst (
    @echo Extracting modcache
    Powershell -C "7z x c:\mnt\modcache.tar.zst -oc:\mnt"
    Powershell -C "7z x c:\mnt\modcache.tar -oc:\modcache"
    del /f c:\mnt\modcache.tar.zst
    del /f c:\mnt\modcache.tar
    @echo Modcache extracted
) else (
    @echo modcache.tar.zst not found, dependencies will be downloaded
)
