=============
Release Notes
=============

6.0.0-beta.8
============

New Features
------------

- Logs-agent now runs as a goroutine in the main agent process

- All docker event subscribers are multiplexed in one connection, reduces stress on the docker daemon

- The Agent can find relevant listeners on its host by using the "auto" listener, for now only docker is supported

- The Docker label AD provider now watches container events and
  only updates when containers start/die to save resources

- Add a new option, `force_tls_12`, to the agent configuration to force the
  TLS version to 1.2 when contactin Datatog.

- Reno and releasenotes are now mandatory. A test will fail if no
  releasenotes where added/updated to the PR. A 'noreno' label can be added
  to the PR to skip this test.


Bug Fixes
---------

- [logs] Fix an issue when the hostname was not provided in datadog.yaml: the logs-agent logic uses the same hostname as the main agent

- [logs] Trim spaces from single lines

- Fix missing fields in forwarder logging entries

- Fix RancherOS cgroup mountpoint detection

- [linux packaging] Fix missing dd-agent script after upgrade, the fix will take effect on a fresh install of '>= beta.8' or upgrade from '>= beta.8'

- [logs] Do not send empty logs with multilines

- [flare] Fix command on Windows by fixing path of collected log files

- Fix path of logs collected by flare on Windows, was breaking flare command


Other Notes
-----------

- Remove resversion handling from podwatcher, as it's unused

- Refactor corecheck boilerplate in CheckBase

- [flare] Rename config file dumped from memory

