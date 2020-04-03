if exist %APPDATA%\pip rd /s/q %APPDATA%\pip
mkdir %APPDATA%\pip
echo "[global]" > %APPDATA%\pip\pip.ini
echo "extra-index-url = https://%ARTIFACTORY_USER%:%ARTIFACTORY_PASSWORD%@%ARTIFACTORY_URL%/simple" >> %APPDATA%\pip\pip.ini
