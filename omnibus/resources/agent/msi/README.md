# Windows Installation Guide

Welcome to the Datadog installer for Windows.

## Basic installation

The Datadog installer for Windows is a Microsoft Installer (MSI) package containing everything required to install the Datadog Agent.

## Default installation

The most basic installation is performed by downloading the installer and running the command

* (cmd shell) `start /wait msiexec /q /i datadog-agent-6-latest.amd64.msi`
* (powershell) `Start-Process msiexec -ArgumentList '/q /i datadog-agent-6-latest.amd64.msi'` -Wait

This will result in all files being installed, but the agent will _not_ be functional.  This is because the agent won't run without an API_KEY.  This can be accomplished by editing the configuration file, `c:\programdata\datadog\datadog.yaml`.

## Installation with options

There are many items that can be preconfigured on the command line.  Each configuration item is added as an install property to the command line.  

* (cmd shell) `msiexec /qn /i datadog-agent-6-latest.amd64.msi APIKEY="abcdefabcdefabcdefabcdefabcdefab" HOSTNAME="my_hostname" TAGS="mytag1,mytag2"`
* (powershell) `Start-Process msiexec -ArgumentList 'datadog-agent-6-latest.amd64.msi APIKEY="abcdefabcdefabcdefabcdefabcdefab" HOSTNAME="my_hostname" TAGS="mytag1,mytag2"'`

The above will install the agent, and preconfigure the configuration file with the APIKEY, as well as setting the hostname and tags.

## Configuration Options

The following configuration command line options are available when installing the agent.
* APIKEY="_string_"
  * Assigns the Datadog API KEY to string in the configuration file
* TAGS="_string_"
  * _string_ is a comma separated list of tags to assign in the configuration file
* HOSTNAME="_string_"
  * configures the hostname reported by the Datadog agent to _string_.  Overrides any hostname calculated at runtime.
* LOGS_ENABLED="_string_"
  * enables (_string_ is **true**) or explicitly disables (_string_ is **false**) the log collection feature in the configuration file.  Logs is disabled by default.
* APM_ENABLED="_string_"
  * explicitly enables (_string_ is **true**) or disables (_string_ is **false**) the APM agent in the configuration file.  APM is enabled by default
* PROCESS_ENABLED="_string_"
  * enables (_string_ is **true**) or explicitly disables (_string_ is **false**) the process agent in the configuration file.  The process agent is disabled by default.
* CMD_PORT="_number_"
  * Number is a valid port number between 0 and 65534.  The datadog agent uses port 5001 by default for it's control API.  If that port is already in use by another program, the default may be overridden here
* PROXY_HOST="_string_"
* PROXY_PORT="_number_"
* PROXY_USER="_string_"
* PROXY_PASSWORD="_string_"
  * Takes the combination of the provided PROXY_* options, and creates the HTTP proxy configuration which the agent will use to report metrics back to the Datadog service.
* DD_URL="_string_"
  * Sets the **dd_url** variable in datadog.yaml to _string_. 
* LOGS_DD_URL="_string_"
  * Sets the **dd_url** variable in the **logs_config** section in datadog.yaml to _string_. 
* PROCESS_DD_URL="_string_"
  * Sets the **process_dd_url** variable in the **process_config** section in datadog.yaml to _string_. 
* TRACE_DD_URL="_string_"
  * Sets the **apm_dd_url** variable in the **apm_config** section in datadog.yaml to _string_. 
