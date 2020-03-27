
# Disable infra checks that would report metrics from the container (instead of the host)
Remove-Item C:\ProgramData\Datadog\conf.d\disk.d\conf.yaml.default
Remove-Item C:\ProgramData\Datadog\conf.d\network.d\conf.yaml.default
Remove-Item C:\ProgramData\Datadog\conf.d\winproc.d\conf.yaml.default
Remove-Item C:\ProgramData\Datadog\conf.d\file_handle.d\conf.yaml.default

# TODO: Conditionally enable the IO check if the host drives are mounted in the container
Remove-Item C:\ProgramData\Datadog\conf.d\io.d\conf.yaml.default

