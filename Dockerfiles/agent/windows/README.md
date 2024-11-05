How to build the Agent docker image
===================================

1. Build the Agent binaries

From the root of the repository, run the following command:

```
docker run --rm -it -v "${pwd}:c:\mnt" -e OMNIBUS_TARGET=main -e MAJOR_VERSION=7 -e RELEASE_VERSION=nightly -e PY_RUNTIMES=3 datadog/agent-buildimages-windows_x64:1809 c:\mnt\tasks\winbuildscripts\buildwin.bat
```

The build artifacts will be in `omnibus\pkg`.

2. Build the entrypoint

From the root of the repository, run the following command:

```
docker run --rm -it -v "${pwd}:c:\mnt" datadog/agent-buildimages-windows_x64:1809 c:\mnt\Dockerfiles\agent\windows\entrypoint\build.bat
```

The build artifact will be in `build-out`.

3. Copy everything to the correct location

```
Copy-Item .\build-out\entrypoint.exe .\Dockerfiles\agent\
Copy-Item .\omnibus\pkg\datadog-agent-*-x86_64.zip -Destination .\Dockerfiles\agent\datadog-agent-latest.amd64.zip
```

4. Build the container

From the `Dockerfiles\agent\` folder, run either of the following commands:

a. To build the containerized Agent from a Nano Windows base image:
```
# Build nano image
docker build -t mycustomagent --build-arg BASE_IMAGE=mcr.microsoft.com/powershell:lts-nanoserver-1809 --build-arg WITH_JMX=false --build-arg VARIANT=1809 -f .\windows\amd64\Dockerfile .
```

a. To build the containerized Agent from a Core Windows base image:
```
# Build core image
docker build -t mycustomagent --build-arg BASE_IMAGE=mcr.microsoft.com/powershell:windowsservercore-1809 --build-arg WITH_JMX=false --build-arg VARIANT=1809 -f .\windows\amd64\Dockerfile .
```

If you need JMX, change `WITH_JMX` to `true`.
