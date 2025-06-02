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
  3. Return
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
   1. We try to fetch the EC2 instance ID if one of the following condition is met:
      - we're running on a ECS instance.
      - `ec2_prioritize_instance_id_as_hostname` config setting is set to `true`.
      - the previously detected hostname is a default EC2 hostname.
      - `ec2_prefer_imdsv2` is set to `true`.
      - `ec2_imdsv2_transition_payload_enabled` is set to `true`.
   2. Else
      1. If the previously detected hostname is a Windows default hostname for EC2:
         1. We fetch the instance ID and log a message about using `ec2_use_windows_prefix_detection` if it's
            different than the previously detected hostname.

# Hostnames and Aliases on EC2

Determining the hostname and aliases on EC2 is particularly complex due to the interplay between
AWS's Instance Metadata Service (IMDS) versions and Agent configuration.

EC2 supports two versions of IMDS: v1 and v2. IMDSv1 can be disabled via the EC2 API, while IMDSv2
enforces a hop limit for requests. By default, IMDSv2 will not respond to requests that originate
more than one network hop away. This becomes problematic when the Agent runs inside a container
without host networking, introducing an extra hop and potentially blocking access to IMDSv2.

The Agent’s behavior is also influenced by two configuration flags: `ec2_prefer_imdsv2` and
`ec2_imdsv2_transition_payload_enabled`. Depending on the combination of these flags and the IMDS
availability, the Agent may take different paths to resolve the instance ID and determine the
appropriate hostname and aliases.

IMDS configuration options are:

- "none" means IMDS is entirely disabled;
- "v2 only" means that IMDSv1 is disabled,
- "v1+v2" is the default setting, with both versions available

## Hostname Resolution Matrix

### Hops Needed = 1

#### IMDS = none

| `ec2_prefer_imdsv2` | `ec2_imdsv2_transition_payload_enabled` | Hostname | Aliases |
| ------------------- | --------------------------------------- | -------- | ------- |
| false               | true                                    | os       | none    |
| true                | true                                    | os       | none    |
| false               | false                                   | os       | none    |
| true                | false                                   | os       | none    |

#### IMDS = v2 only

| `ec2_prefer_imdsv2` | `ec2_imdsv2_transition_payload_enabled` | Hostname   | Aliases    |
| ------------------- | --------------------------------------- | ---------- | ---------- |
| false               | true                                    | aws (i-..) | aws (i-..) |
| true                | true                                    | aws (i-..) | aws (i-..) |
| false               | false                                   | os         | aws (i-..) |
| true                | false                                   | aws (i-..) | aws (i-..) |

#### IMDS = v1+v2

| `ec2_prefer_imdsv2` | `ec2_imdsv2_transition_payload_enabled` | Hostname   | Aliases    |
| ------------------- | --------------------------------------- | ---------- | ---------- |
| false               | true                                    | aws (i-..) | aws (i-..) |
| true                | true                                    | aws (i-..) | aws (i-..) |
| false               | false                                   | aws (i-..) | aws (i-..) |
| true                | false                                   | aws (i-..) | aws (i-..) |

---

### Hops Needed = 2

#### IMDS = none

| `ec2_prefer_imdsv2` | `ec2_imdsv2_transition_payload_enabled` | Hostname | Aliases |
| ------------------- | --------------------------------------- | -------- | ------- |
| false               | true                                    | os       | none    |
| true                | true                                    | os       | none    |
| false               | false                                   | os       | none    |
| true                | false                                   | os       | none    |

#### IMDS = v2 only

| `ec2_prefer_imdsv2` | `ec2_imdsv2_transition_payload_enabled` | Hostname | Aliases |
| ------------------- | --------------------------------------- | -------- | ------- |
| false               | true                                    | os       | none    |
| true                | true                                    | os       | none    |
| false               | false                                   | os       | none    |
| true                | false                                   | os       | none    |

#### IMDS = v1+v2

| `ec2_prefer_imdsv2` | `ec2_imdsv2_transition_payload_enabled` | Hostname   | Aliases    |
| ------------------- | --------------------------------------- | ---------- | ---------- |
| false               | true                                    | os         | aws (i-..) |
| true                | true                                    | os         | aws (i-..) |
| false               | false                                   | aws (i-..) | aws (i-..) |
| true                | false                                   | os         | aws (i-..) |

---

### Notes

- **aws (i-..)**: AWS-assigned hostname format including the EC2 instance ID.
- **os**: The machine’s operating system-provided hostname.
- **Aliases none**: No alternative hostname aliases available.
