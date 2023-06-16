if exist c:\mnt\modcache_tools_%CI_PIPELINE_ID%.tar.gz (
    @echo Extracting modcache_tools
    Powershell -C "7z x c:\mnt\modcache_tools_%CI_PIPELINE_ID%.tar.gz -oc:\mnt"
    REM Use -aoa to allow overwriting existing files
    REM This shouldn't have any negative impact: since modules are
    REM stored per version and hash, files that get replaced will
    REM get replaced by the same files
    Powershell -C "7z x c:\mnt\modcache_tools_%CI_PIPELINE_ID%.tar -oc:\modcache -aoa"
    del /f c:\mnt\modcache_tools_%CI_PIPELINE_ID%.tar.gz
    del /f c:\mnt\modcache_tools_%CI_PIPELINE_ID%.tar
    @echo modcache_tools extracted
) else (
    @echo modcache_tools_%CI_PIPELINE_ID%.tar.gz not found, tooling dependencies will be downloaded
)
