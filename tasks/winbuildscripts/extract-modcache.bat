if exist c:\mnt\modcache.tar.gz (
    @echo Extracting modcache in dedicated repository
    if not exist c:\cache (
        mkdir "c:\cache"
    )
    move "c:\mnt\modcache.tar.gz" "c:\cache\modcache.tar.gz"
    if errorlevel 1 (
        @echo Failed to move modcache
    ) else (
        @echo Successfully moved modcache
    )
    Powershell -C "7z x c:\cache\modcache.tar.gz -oc:\cache"
    Powershell -C "7z x c:\cache\modcache.tar -oc:\modcache"
    del /f c:\cache\modcache.tar.gz
    del /f c:\cache\modcache.tar
    @echo Modcache extracted
) else (
    @echo modcache.tar.gz not found, dependencies will be downloaded
)
