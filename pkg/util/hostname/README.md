# Hostname detection logic

The following doc describes the logic the Agent goes through to choose a hostname. The hostname uniquely identifies a
host in the backend. It must be consistent between agent restart and unique to the host.

The hostname is cached for the entire life span of the Agent.

## Get

`Get` will return the hostname detected and `GetWithProvider` will return the hostname with the provider used to fetch
it. Those functions are also in charge of updating `goexpvar` and `inventories` with the correct information, so the
`status` page and metadata are updated correctly.

We have a list of providers that the logic will go through. The order of that list matters as it mimics what was in
Agent V5 and the previous versions of the hostname detection. We need to be backward compatible as changing the hostname
detected by the Agent might create a new host in the backend and break dashboards, monitors, ...

Each `provider` in `providerCatalog` has:
- `name`: unique name to identify it
- `cb`: a call back to fetch the hostname. The callback will take the hostname previously detected by other providers in
  the `providerCatalog` list. Provider like `aws` act differently based on the result from other providers.
- `stopIfSuccessful`: should we stop going down the `providerCatalog` list if the provider is successful
- `expvarName`: the provider name to use in expvar. This is then used by the status page.

When calling a provider we always:
- If successful and `stopIfSuccessful` is set to true:
    1. Update cache
    2. Set hostname provider in `goexpvar` and `inventories`
    4. Return
- If unsuccessful: we export the error return to expvar to be displayed by the status page

### The current logic

1. If `hostname` is set to a valid hostname we use it. If `hostname_force_config_as_canonical` is `false` and the hostname
   has the default prefixes used on EC2 we log a warning about non canonical hostname.
2. If `hostname_file` is set to a valid, non-empty file we use it's content as hostname. If
   `hostname_force_config_as_canonical` is `false` and the hostname has the default prefixes used on EC2 we log a warning
   about non canonical hostname.
3. If running on **Fargate**: we set an empty hostname as the idea of a host doesn't exist. We **DO NOT** set hostname
   provider in `goexpvar` and `inventories`
4. **GCE**: if we can fetch a hostname from the GCE metadata API we use it.
5. **Azure**: if we can fetch a hostname from the Azure metadata API we use it.

**The following providers behavior are linked to each other**

The idea is that we will use the OS or FQDN hostname unless it's the default hostname from EC2, in which case we will
try to use the EC2 instanceID. The logic around EC2 is different from other providers. This dates back to the Agent V5
and we have to be backward compatible or the hostname would change on an Agent upgrade which might break dashboards,
monitors and more.

This means that we don't stop when a provider found a hostname but continue down the list. The value from the previous
provider is pass to the next one unless it returned an error.

The notion of `isOSHostnameUsable` means:
- If we're not running in a containerized environment -> True
- Else if we can fetch the container UTS mode and it's not `host` or 'unknown' -> False
- Else if we're on k8s and running inside a container without `hostNetwork` set to true -> False
- Else -> True

6. **FQDN**
    1. If `isOSHostnameUsable` is false we return an error
    2. If `hostname_fqdn` config setting is set to true we fetch the FQDN:
        1. On Linux we use `/bin/hostname -f`
        2. On Windows we use `golang.org/x/sys/windows:GetHostByName`
    3. Else we return an error
7. **CONTAINER**
    1. If we're running in a containerized environment we try to get the hostname from, in order: `kube_apiserver`,
       `docker` and `kubelet`.
8. **OS**
    1. If `isOSHostnameUsable` is true and previous providers didn't detect a hostname we use `os.Hostname()`
9. **EC2**
    1. If we're running on a ECS instance or `ec2_prioritize_instance_id_as_hostname` config setting is set to true or
       the previously detected hostname is a default EC2 hostname: we try to fetch the EC2 instance ID.
    2. Else
        1. If the previously detected hostname is a Windows default hostname for EC2:
            1. We fetch the instance ID and log a message about using `ec2_use_windows_prefix_detection` if it's
               different than the previously detected hostname.

# Hostnames and Aliases on EC2

Determining hostname and aliases on EC2 is particularly complicated.
EC2 offers two versions of its metadata service, v1 and v2.
The v1 interface can be disabled via the EC2 API.
However, the v2 interface verifies the IP hop count in requests, and by default will not respond to TCP connections from more than one hop away.
This causes a problem when the Agent tries to access IMDSv2 within a container that does not use the host's network, which introduces a second hop.
Finally, the `ec2_prefer_imdsv2` config flag affects the agent's behavior.

The results are as follows:

| *IMDS*    | *ec2_prefer_imdsv2*   | *hops*    | _hostname_    | _aliases_     |
|--------   |---------------------  |--------   |-------------  |------------   |
| none      | false                 | 1         | os            | none          |
| none      | true                  | 1         | os            | none          |
| v2 only   | false                 | 1         | os            | none          |
| v2 only   | true                  | 1         | aws (i-..)    | aws (i-..)    |
| v1+v2     | false                 | 1         | aws (i-..)    | aws (i-..)    |
| v1+v2     | true                  | 1         | aws (i-..)    | aws (i-..)    |
| none      | false                 | 2+        | os            | none          |
| none      | true                  | 2+        | os            | none          |
| v2 only   | false                 | 2+        | os            | none          |
| v2 only   | true                  | 2+        | os            | none          |
| v1+v2     | false                 | 2+        | os            | aws (i-..)    |
| v1+v2     | true                  | 2+        | os            | aws (i-..)    |

 * The first column describes the EC2 IMDS configuration: "none" means IMDS is entirely disabled; "v2 only" means that IMDSv1 is disabled, and "v1+v2" is the default setting with both versions available
 * The second column is the `ec2_prefer_imdsv2` configuration value.
 * The third column contains the number of IP hops between the Agent and the IMDS, assuming the default limit of 1.
 * The fourth column describes the selected hostname source
 * The fifth column gives the discovered host aliased, if any (determined in pkg/util/ec2).
