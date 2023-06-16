if exist c:\mnt\modcache_%CI_PIPELINE_ID%.tar.gz (
    @echo Extracting modcache
    Powershell -C "7z x c:\mnt\modcache_%CI_PIPELINE_ID%.tar.gz -oc:\mnt"
    Powershell -C "7z x c:\mnt\modcache_%CI_PIPELINE_ID%.tar -oc:\modcache"
    del /f c:\mnt\modcache_%CI_PIPELINE_ID%.tar.gz
    del /f c:\mnt\modcache_%CI_PIPELINE_ID%.tar
    @echo Modcache extracted
) else (
    @echo modcache_%CI_PIPELINE_ID%.tar.gz not found, dependencies will be downloaded
)
