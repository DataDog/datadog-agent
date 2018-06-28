# Difference in hostname resolution between Agent v5 and Agent v6 (<v6.4)

In some cases, it is possible to see a difference in the hostname that’s reported by your Agent when upgrading from Agent v5 to Agent v6 (for versions < 6.4). 

To resolve the system hostname the Agent 5 uses the `hostname -f` command while the Agent v6 (for versions < 6.4) uses the Golang API `os.Hostname()`. 

The most common issue this can create is to change your hostname from a Fully-Qualified Domain Name (FQDN, ex. sub.domain.tld) to a short hostname (ex. sub). 

Starting from the Agent v6.3 a configuration flag called `hostname_fqdn` has been introduced that allows the Agent v6 to have the same behavior as Agent v5. This flag is disabled by default on version 6.3 and enabled by default in version 6.4.

You are not affected if any of the following is true :
- You are setting the Agent hostname in datadog.conf, datadog.yaml or through the DD_HOSTNAME environment variable
- You are running on Windows
- You are running the Agent in a container with access to the Docker or Kubernetes API
- You are running the Agent in GCE
- `cat /proc/sys/kernel/hostname` and `hostname -f` output the same hostname

Starting with v6.3.0, the Agent will log a warning (`DEPRECATION NOTICE: The agent resolved your hostname as <hostname>. However starting from version 6.4, it will be resolved as <fqdn> by default. To enable the behavior of 6.4+, please enable the `hostname_fqdn` flag in the configuration.`) if you’re affected by this change.

Recommended action:

If you’re upgrading from Agent v5 to Agent v < 6.3: Hardcode your hostname in the agent configuration.

If you’re upgrading from Agent v5 to Agent v6.3: enable the `hostname_fqdn` option in the Agent v6 configuration to ensure that you will keep the same hostname.

If you’re upgrading from Agent v5 to Agent v > 6.3: you don’t need to take any action.
