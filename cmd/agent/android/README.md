# Datadog Agent 6 on Android

## Requirements

-  android sdk and ndk (installed with android studio for example)
- `gomobile` (through `go get golang.org/x/mobile/cmd/gomobile`) [godoc](https://godoc.org/golang.org/x/mobile/cmd/gomobile)
- jdk (warning: on macos `brew cask install java8`, jdk10 provided by Oracle doesn't work)


## Before building

- `ANDROID_HOME` environment variable set to the android sdk path (if installed with android studio:  `$HOME/Android/Sdk` on linux and `$HOME/Library/Android/sdk` on macos).
- initialize gomobile with `gomobile init -ndk /path/to/ndk` (ndk is at `$ANDROID_HOME/ndk-bundle` if installed with android studio).

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

- use `inv android.launchservice <api_key> --hostname=<hostname>` to start the service on the android device.
- use `inv android.stopservice` to stop the agent.