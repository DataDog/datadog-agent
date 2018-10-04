_Edit 2018-31-08 : The default value of the new `hostname_fqdn` flag is planned to change in 6.6.0 instead of 6.4.0 to give users more time to take it into account._

# Difference in hostname resolution between Agent v5 and Agent v6 (<v6.6)

In some cases, it is possible to see a difference in the hostname that’s reported by your Agent when upgrading from Agent v5 to Agent v6 (for versions < 6.6). 

To resolve the system hostname the Agent 5 uses the `hostname -f` command while the Agent v6 (for versions < 6.6) uses the Golang API `os.Hostname()`. 

On upgrades from Agent v5 to Agent v6 (<6.6.0), this may make the Agent hostname change from a Fully-Qualified Domain Name (FQDN, ex. sub.domain.tld) to a short hostname (ex. sub). 

Starting from the Agent v6.3 a configuration flag called `hostname_fqdn` has been introduced that allows the Agent v6 to have the same behavior as Agent v5. This flag is disabled by default on version 6.3 and enabled by default in version 6.6.

## Determine if you're affected

Starting with v6.3.0, the Agent will log a warning (`DEPRECATION NOTICE: The agent resolved your hostname as <hostname>. However starting from version 6.6, it will be resolved as <fqdn> by default. To enable the behavior of 6.6+, please enable the `hostname_fqdn` flag in the configuration.`) if you’re affected by this change.

You are not affected if any of the following is true :
- You are running the Agent in GCE
- You are running on Windows
- You are setting the Agent hostname in datadog.conf, datadog.yaml or through the DD_HOSTNAME environment variable
- You are running the Agent in a container with access to the Docker or Kubernetes API
- `cat /proc/sys/kernel/hostname` and `hostname -f` output the same hostname

## Recommended action

If you're affected by this change, we recommend that you take the following action when you upgrade your Agent:

- Upgrading from Agent v5 to Agent v < 6.3: Hardcode your hostname in the agent configuration.

- Upgrading from Agent v5 to Agent >= v6.3: enable the `hostname_fqdn` option in the Agent v6 configuration to ensure that you will keep the same hostname.

- Upgrading from Agent v5 to Agent v >= 6.6 (future): you don’t need to take any action.

- Upgrading from Agent v6 < 6.6 to Agent >= v6.6: If you wish to keep the behavior of Agent v6 (<6.6) for now, set hostname_fqdn to false. We recommend you switch hostname_fqdn to true whenever possible.

