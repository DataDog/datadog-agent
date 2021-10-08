# [BETA] Datadog Agent 6 on Android

**This feature is in beta and its options or behavior might break between minor or bugfix releases of the Agent.**

## Requirements

-  android sdk and ndk (installed with android studio for example)
- jdk (warning: on macos `brew cask install java8`, jdk10 provided by Oracle doesn't work)


## Before building

- `ANDROID_HOME` environment variable set to the android sdk path (if installed with android studio:  `$HOME/Android/Sdk` on linux and `$HOME/Library/Android/sdk` on macos).
- `ANDROID_NDK_HOME` environment variable set to the android bdk path (ndk is at `$ANDROID_HOME/ndk-bundle` if installed with android studio).
- install go tools: `invoke install-tools`
- install and initialize gomobile with `invoke deps-vendored`.

`datadog-agent` is installed as a system service on Android and has to be signed with the platform key.

- The keys used to sign the emulator images are available in the [android source tree](https://android.googlesource.com/platform/build/+/master/target/product/security/).
- `apksigner.jar` is a java tool to sign the apk it is located in `$ANDROID_HOME/build-tools/<version>/lib/apksigner.jar`.

`apksigner.jar`, `platform.pk8` and `platform.x509.pem` should be put in the root of the datadog-agent repo.

## Using the android emulator

Create a virtual android device from any image without the google APIs, the google images are signed with another key, thus the agent can't be installed.


## Build and install

- use `inv android.build` to build the apk.
- use `inv android.sign-apk` to sign the apk.
- use `inv android.install` to build, sign and install the agent on a currently connected device.

Note: These commands assume `adb` is in your path, you can get it from `$ANDROID_HOME/platform-tools/adb`.


## Launch the service

- use `inv android.launchservice <api_key> --hostname=<hostname> --tags=<optional comma separated list of tags>` to start the service on the android device.
- use `inv android.stopservice` to stop the agent.

Note: these commands assume `adb` is in your path as well.
