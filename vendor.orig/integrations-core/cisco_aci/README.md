# CiscoACI Integration

## Overview

The Cisco ACI Integration lets you:

* Track the state and health of your network
* Track the capacity of your ACI
* Monitor the switches and controllers themselves

## Setup
### Installation

The Cisco ACI check is packaged with the Agent, so simply [install the Agent][1] on a server within your network.

### Configuration

1. Edit the `cisco_aci.d/conf.yaml` file, in the `conf.d/` folder at the root of your Agent's directory.  
    See the [sample cisco_aci.d/conf.yaml][2] for all available configuration options:  

    ```yaml
      init_config:
          # This check makes a lot of API calls
          # it could sometimes help to add a minimum collection interval
          # min_collection_interval: 180
      instances:
          - aci_url: localhost # the url of the aci controller
            username: datadog
            pwd: datadog
            timeout: 15
            # if it's an ssl endpoint that doesn't have a certificate, use this to ensure it can still connect
            ssl_verify: True
            tenant:
              - WebApp
              - Database
              - Datadog
    ```

2. [Restart the Agent][3] to begin sending Cisco ACI metrics to Datadog.

### Validation

[Run the Agent's `status` subcommand][4] and look for `cisco_aci` under the Checks section.

## Data Collected
### Metrics
See [metadata.csv][5] for a list of metrics provided by this integration.

### Events
The Cisco ACI check sends tenant faults as events.

### Service Checks

`cisco_aci.can_connect`:

Returns CRITICAL if the Agent cannot connect to the Cisco ACI API to collect metrics, otherwise OK.

## Troubleshooting
Need help? Contact [Datadog Support][6].

## Further Reading
Learn more about infrastructure monitoring and all our integrations on [our blog][7]

[1]: https://app.datadoghq.com/account/settings#agent
[2]: https://github.com/DataDog/integrations-core/blob/master/cisco_aci/conf.yaml.example
[3]: https://docs.datadoghq.com/agent/faq/agent-commands/#start-stop-restart-the-agent
[4]: https://docs.datadoghq.com/agent/faq/agent-commands/#agent-status-and-information
[5]: https://github.com/DataDog/integrations-core/blob/master/cisco_aci/metadata.csv
[6]: http://docs.datadoghq.com/help/
[7]: https://www.datadoghq.com/blog/
