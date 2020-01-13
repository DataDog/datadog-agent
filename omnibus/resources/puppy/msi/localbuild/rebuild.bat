rem
@setlocal
@set WD=%CD%
cd %~dp0

copy ..\source.wxs.erb source.wxs
xcopy /y/e/s/i ..\assets\*.* resources\assets
REM run HEAT on c:\opt\datadog-agent
REM
heat.exe dir "c:\opt\datadog-agent" -nologo -srd -sreg -gg -cg ProjectDir -dr PROJECTLOCATION -var "var.ProjectSourceDir" -out "project-files.wxs"

REM 
REM run HEAT on the extras
REM
for /D %%D in (c:\omnibus-ruby\src\etc\datadog-agent\extra_package_files\*.*) do (
    heat.exe dir %%D -nologo -srd -gg -cg Extra%%~nD -dr %%~nD -var "var.Extra%%~nD" -out "extra-%%~nD.wxs"
    set CANDLE_VARS=%CANDLE_VARS% -dExtra%%~nD="%%D"
    set WXS_LIST=%WX_LIST% extra-%%~nD.wxs
    set WIXOBJ_LIST=%WIXOBJ_LIST% extra-%%~nD.wixobj
)

@set wix_extension_switches=-ext WixUtilExtension
candle -arch x64 %wix_extension_switches% -dProjectSourceDir="c:\opt\datadog-agent" -dExtraEXAMPLECONFSLOCATION="C:\omnibus-ruby\src\etc\datadog-agent\extra_package_files\EXAMPLECONFSLOCATION" project-files.wxs %WXS_LIST% source.wxs

if not "%ERRORLEVEL%" == "0" goto :done
light -ext WixUIExtension -ext WixBalExtension %wix_extension_switches% -cultures:en-us -loc localization-en-us.wxl project-files.wixobj source.wixobj %WIXOBJ_LIST% -out ddagent.msi

@echo light returned code %ERRORLEVEL%


:done
cd %WD%