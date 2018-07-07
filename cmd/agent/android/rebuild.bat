call gradlew.bat build
if not "%ERRORLEVEL%" == "0" goto :EOF
java -jar ..\..\..\signapk.jar ..\..\..\platform.x509.pem ..\..\..\platform.pk8 app\build\outputs\apk\debug\app-debug.apk dd-agent-signed.apk
if not "%ERRORLEVEL%" == "0" goto :EOF
\devtools\platform-tools\adb install -r dd-agent-signed.apk