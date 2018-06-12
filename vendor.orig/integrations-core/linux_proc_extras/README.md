# Linux_proc_extras Integration

## Overview
Get metrics from linux_proc_extras service in real time to:

* Visualize and monitor linux_proc_extras states
* Be notified about linux_proc_extras failovers and events.

## Setup
### Installation

The Linux_proc_extras check is included in the [Datadog Agent][1] package, so you don't need to install anything else on your servers.

### Configuration

Edit the `linux_proc_extras.d/conf.yaml` file, in the `conf.d/` folder at the root of your Agent's directory. See the [sample linux_proc_extras.d/conf.yaml][2] for all available configuration options.

### Validation

[Run the Agent's `status` subcommand][3] and look for `linux_proc_extras` under the Checks section.

## Data Collected
### Metrics
The Linux proc extras check does not include any metrics at this time.

### Events
The Linux proc extras check does not include any events at this time.

### Service Checks
The Linux proc extras check does not include any service checks at this time.

## Troubleshooting

Need help? Contact [Datadog Support][4].

## Further Reading
Learn more about infrastructure monitoring and all our integrations on [our blog][5]


[1]: https://app.datadoghq.com/account/settings#agent
[2]: https://github.com/DataDog/integrations-core/blob/master/linux_proc_extras/conf.yaml.example
[3]: https://docs.datadoghq.com/agent/faq/agent-commands/#agent-status-and-information
[4]: http://docs.datadoghq.com/help/
[5]: https://www.datadoghq.com/blog/
