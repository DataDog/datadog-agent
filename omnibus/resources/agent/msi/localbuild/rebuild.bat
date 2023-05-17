rem
@setlocal
@set WD=%CD%
cd %~dp0

if NOT DEFINED OMNIBUS_BASE_DIR set OMNIBUS_BASE_DIR=C:\omnibus-ruby

REM Copy few resource files locally, source.wxs AS IS and
REM localization-en-us.wxl.erb replace template value currently via "Agent",
REM unless localization-en-us.wxl already exhists (e.g. from previous copy)
copy ..\source.wxs.erb source.wxs

PowerShell -Command "Get-Content ..\localization-en-us.wxl.erb | %%{$_ -replace '<%%= friendly_name %%>', 'Agent'} | Out-File -Encoding utf8 'localization-en-us.wxl'"


xcopy /y/e/s/i ..\assets\*.* resources\assets

REM first create zip file if it's not already there (of the embedded directory)
REM this assumes always A7 for now

REM run HEAT on c:\opt\datadog-agent
REM
if not exist c:\opt\datadog-agent\embedded3.7z (
    @echo Zip file not present, creating
    if not exist c:\opt\datadog-agent\embedded3 (
        @echo no embedded3 directory, can't make zip
        exit /b 5
    )
    7z a -mx=5 -ms=on c:\opt\datadog-agent\embedded3.7z c:\opt\datadog-agent\embedded3
    rd /s/q c:\opt\datadog-agent\embedded3
) else (
    @echo zip file present, using existing zip
)
heat.exe dir "c:\opt\datadog-agent" -nologo -srd -sreg -gg -cg ProjectDir -dr PROJECTLOCATION -var "var.ProjectSourceDir" -out "project-files.wxs"

REM 
REM run HEAT on the extras
REM
for /D %%D in (%OMNIBUS_BASE_DIR%\src\etc\datadog-agent\extra_package_files\*.*) do (
    heat.exe dir %%D -nologo -srd -gg -cg Extra%%~nD -dr %%~nD -var "var.Extra%%~nD" -out "extra-%%~nD.wxs"
    set CANDLE_VARS=%CANDLE_VARS% -dExtra%%~nD="%%D"
    set WXS_LIST=%WX_LIST% extra-%%~nD.wxs
    set WIXOBJ_LIST=%WIXOBJ_LIST% extra-%%~nD.wixobj
)

@set wix_extension_switches=-ext WixUtilExtension
candle -arch x64 %wix_extension_switches% -dProjectSourceDir="c:\opt\datadog-agent" -dExtraEXAMPLECONFSLOCATION="%OMNIBUS_BASE_DIR%\src\etc\datadog-agent\extra_package_files\EXAMPLECONFSLOCATION" project-files.wxs %WXS_LIST% source.wxs

if not "%ERRORLEVEL%" == "0" goto :done
light -ext WixUIExtension -ext WixBalExtension %wix_extension_switches% -cultures:en-us -loc localization-en-us.wxl project-files.wixobj source.wixobj %WIXOBJ_LIST% -out ddagent.msi

@echo light returned code %ERRORLEVEL%


:done
cd %WD%