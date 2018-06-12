# Agent Check: Apache Web Server
{{< img src="integrations/apache/apachegraph.png" alt="apache graph" responsive="true" popup="true">}}
## Overview

The Apache check tracks requests per second, bytes served, number of worker threads, service uptime, and more.

## Setup
### Installation

The Apache check is packaged with the Agent. To start gathering your Apache metrics and logs, you need to:

1. [Install the Agent][1] on your Apache servers.

2. Install `mod_status` on your Apache servers and enable `ExtendedStatus`.

### Configuration

1. Edit the `apache.d/conf.yaml` file in the `conf.d/` folder at the root of your Agent's directory to start collecting your Apache [metrics](#metric-collection) and [logs](#log-collection).
  See the [sample apache.d/conf.yaml][2] for all available configuration options.

2. [Restart the Agent][3]

#### Metric Collection

1. Add this configuration block to your `apache.d/conf.yaml` file to start gathering your [Apache Metrics](#metrics):

    ```yaml
    init_config:

    instances:
        apache_status_url: http://example.com/server-status?auto
    #   apache_user: example_user # if apache_status_url needs HTTP basic auth
    #   apache_password: example_password
    #   disable_ssl_validation: true # if you need to disable SSL cert validation, i.e. for self-signed certs
    ```
    Change the `apache_status_url` parameter value and configure it for your environment.
    See the [sample apache.d/conf.yaml][2] for all available configuration options.

2.  [Restart the Agent][3].

#### Log Collection

**Available for Agent >6.0**

1. Collecting logs is disabled by default in the Datadog Agent, you need to enable it in `datadog.yaml`:

    ```yaml
    logs_enabled: true
    ```

2. Add this configuration block to your `apache.yaml` file to start collecting your Apache Logs:

    ```yaml
      logs:
          - type: file
            path: /var/log/apache2/access.log
            source: apache
            sourcecategory: http_web_access
            service: apache

          - type: file
            path: /var/log/apache2/error.log
            source: apache
            sourcecategory: http_web_access
            service: apache
    ```

    Change the `path` and `service` parameter values and configure them for your environment.
    See the [sample apache.d/conf.yaml][2] for all available configuration options.

3. [Restart the Agent][3].

**Learn more about log collection [in the log documentation][4]**

### Validation

[Run the Agent's `status` subcommand][5] and look for `apache` under the Checks section.

## Data Collected
### Metrics

See [metadata.csv][6] for a list of metrics provided by this check.

### Events
The Apache check does not include any events at this time.

### Service Checks

**apache.can_connect**:

Returns CRITICAL if the Agent cannot connect to the configured `apache_status_url`, otherwise OK.

## Troubleshooting

* [Issues with Apache Integration][7]
* [Apache SSL certificate issues][8]

## Further Reading

* [Monitoring Apache web server performance][9]
* [How to collect Apache performance metrics][10]
* [How to monitor Apache web server with Datadog][11]


[1]: https://app.datadoghq.com/account/settings#agent
[2]: https://github.com/DataDog/integrations-core/blob/master/apache/conf.yaml.example
[3]: https://docs.datadoghq.com/agent/faq/agent-commands/#start-stop-restart-the-agent
[4]: https://docs.datadoghq.com/logs
[5]: https://docs.datadoghq.com/agent/faq/agent-commands/#agent-status-and-information
[6]: https://github.com/DataDog/integrations-core/blob/master/apache/metadata.csv
[7]: https://docs.datadoghq.com/integrations/faq/issues-with-apache-integration
[8]: https://docs.datadoghq.com/integrations/faq/apache-ssl-certificate-issues
[9]: https://www.datadoghq.com/blog/monitoring-apache-web-server-performance/
[10]: https://www.datadoghq.com/blog/collect-apache-performance-metrics/
[11]: https://www.datadoghq.com/blog/monitor-apache-web-server-datadog/
