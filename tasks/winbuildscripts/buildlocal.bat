@echo RELEASE_VERSION %RELEASE_VERSION%
@echo MAJOR_VERSION %MAJOR_VERSION%

REM set up variables for local build.
REM assumes attempting to build A7/x64 nightly
REM assumes target directory is mounted in the container
REM (vs. copied in as in CI build)
if NOT DEFINED RELEASE_VERSION set RELEASE_VERSION=nightly
if NOT DEFINED MAJOR_VERSION set MAJOR_VERSION=7
if NOT DEFINED CI_JOB_ID set CI_JOB_ID=1
if NOT DEFINED TARGET_ARCH set TARGET_ARCH=x64

call %~dp0dobuild.bat
if not %ERRORLEVEL% == 0 @echo "Build failed %ERRORLEVEL%"
