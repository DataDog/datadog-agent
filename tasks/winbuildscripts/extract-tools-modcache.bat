if exist c:\mnt\modcache_tools.tar.gz (
    @echo Extracting modcache_tools in a dedicated repository
    if not exist c:\cache (
        mkdir "c:\cache"
    )
    move "c:\mnt\modcache.tar.gz" "c:\cache\modcache.tar.gz"
    if errorlevel 1 (
        @echo Failed to move modcache
    ) else (
        @echo Successfully moved modcache
    )
    Powershell -C "7z x c:\cache\modcache_tools.tar.gz -oc:\cache"
    REM Use -aoa to allow overwriting existing files
    REM This shouldn't have any negative impact: since modules are
    REM stored per version and hash, files that get replaced will
    REM get replaced by the same files
    Powershell -C "7z x c:\cache\modcache_tools.tar -oc:\modcache -aoa"
    del /f c:\cache\modcache_tools.tar.gz
    del /f c:\cache\modcache_tools.tar
    @echo modcache_tools extracted
) else (
    @echo modcache_tools.tar.gz not found, tooling dependencies will be downloaded
)
