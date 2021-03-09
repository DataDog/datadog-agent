set Python2_ROOT_DIR=c:\opt\datadog-agent\embedded2
set Python3_ROOT_DIR=c:\opt\datadog-agent\embedded3
set CMAKE_INSTALL_PREFIX=c:\opt\datadog-agent\embedded2
SET PATH=%PATH%;%GOPATH%/bin
call ridk enable
REM "inv -e agent.build --exclude-rtloader --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg} --rtloader-root=#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/rtloader --rebuild --no-development --embedded-path=#{install_dir}/embedded --arch #{platform} #{do_windows_sysprobe}", env: env
doskey buildagent=inv -e agent.build --exclude-rtloader --python-runtimes 3 --major-version 7 --rtloader-root=c:/omnibus-ruby/src/datadog-agent/src/github.com/DataDog/datadog-agent/rtloader  --no-development --embedded-path=c:/opt/datadog-agent/embedded --arch x64