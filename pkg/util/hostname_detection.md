# Hostname detection logic

The following doc describes the logic the Agent goes through to choose a hostname. The hostname uniquely identifies a
host in the backend. It must be consistent between agent restart and unique to the host.

The hostname is cached for the entire life span of the Agent.

## GetHostnameData

`GetHostnameData` will return the hostname detected and the provider used to fetch it. This function is also in charge
of updating `goexpvar` and `inventories` with the correct information, so the `status` page and metadata are updated
correctly.

1. Check the **cache**, if we already detected a hostname we use it.
2. **Configuration/Env**:
    1. If `hostname` config setting is set we use it.
        1. Update cache
        2. Set hostname provider in `goexpvar` and `inventories`
        3. If `hostname_force_config_as_canonical` is true and the hostname has the default prefixes used on EC2 we log a
           warning about non canonical hostname.
        4. Return
    2. Else: set error in expvar for provider `configuration/environment` for the status page
3. if `hostname_file` config setting is set:
    1. If `hostname_file` is a valid, non-empty file we use it's content as hostname:
        1. Update cache
        2. Set hostname provider in `goexpvar` and `inventories`
        3. If `hostname_force_config_as_canonical` is true and the hostname has the default prefixes used on EC2 we log a
           warning about non canonical hostname.
        4. Return
    2. Else: set another error in expvar for provider `configuration/environment` for the status page
4. If running on **Fargate**:
    1. We set an empty hostname on Fargate as the idea of host doesn't exist
        1. Update cache
        2. **DO NOT** set hostname provider in `goexpvar` and `inventories`
        3. Return
5. **GCE**
    1. If we can fetch a hostname from the GCE metadata API:
        1. Update cache
        2. Set hostname provider in `goexpvar` and `inventories`
        3. Return
    2. Else: set an error in expvar for provider `gce` for the status page

**The following providers behavior are linked to each other**

The idea is that we will use the OS or FQDN hostname unless it's the default hostname from EC2, in which case we will
try to use the EC2 instanceID. The logic around EC2 is different from other providers. This dates back to the Agent V5
and we have to be backward compatible or the hostname would change on an Agent upgrade which might break dashboards,
monitors and more.

This means that we don't stop when a provider found a hostname but continue down the list, keeping the last successful
hostname in a variable. We will refer to this variable as `lastDetectedHostname`.

The notion of `isOSHostnameUsable` means:
- If we're not running in a containerized environment -> True
- Else if we can fetch the container UTS mode and it's not `host` or 'unknown' -> False
- Else if we're on k8s and running inside a container without `hostNetwork` set to true -> False
- Else -> True

Note on **Azure** provider: it could be higher in this list and return if successful like GCE as it's completely
independent from EC2, FQDN, ...

6. **FQDN**
    1. if `isOSHostnameUsable` is true
        1. we fetch the FQDN:
            1. On Linux we use `/bin/hostname -f`
            2. On Windows we use `golang.org/x/sys/windows:GetHostByName`
        2. If `hostname_fqdn` config setting is set to true and we succeed in fetching a FQDN hostname
            1. Save the hostname in `lastDetectedHostname`
        3. Else: set an error in expvar for provider `fqdn` for the status page
7. If we're not running in a **containerized environment**
    1. We try to get the hostname from, in order: `kube_apiserver`, `docker` and `kubelet` and save it in
       `lastDetectedHostname` if successful
    2. Else: set an error in expvar for provider `container` for the status page
8. **OS**
    1. If `isOSHostnameUsable` is true and steps 6 (`FQDN`) and 7 (`container`) didn't detect any hostname
        1. If we're successful in calling `os.Hostname`: save it in `lastDetectedHostname`
        2. Else: set an error in expvar for provider `os` for the status page
9. **EC2**
    1. If we're running on a ECS instance or `lastDetectedHostname` is a default EC2 hostname or
       `ec2_prioritize_instance_id_as_hostname` config setting is set to true
        1. We try to fetch the EC2 instance ID. If successful:
            1. if `ec2_prioritize_instance_id_as_hostname` is true we:
                1. Update cache
                2. Set hostname provider in `goexpvar` and `inventories`
                3. Return
            2. Else: save the value in `lastDetectedHostname`
        2. Else: set an error in expvar for provider `aws` for the status page
    2. Else
        1. Set an error in expvar for provider `aws` for the status page
        2. if `lastDetectedHostname` is a Windows default one for EC2
            1. Fetch the instance ID if it's different than `lastDetectedHostname` we log a message about using
               `ec2_use_windows_prefix_detection`
10. **Azure**
    1. If we can fetch a hostname from the Azure metadata API:
        1. save the value in `lastDetectedHostname`
    2. Else: set an error in expvar for provider `azure` for the status page
11. We log a warning if the FQDN hostname and OS hostname are not the same and `lastDetectedHostname` is from the OS
    provider.
    1. Warning about pushing users to use `hostname_fqdn` config setting.
12. If `lastDetectedHostname` is empty:
    1. set an error in expvar for provider `all` for the status page
    2. Note: no data is saved to the cache.
    3. return an error
13. Update the cache with `lastDetectedHostname`, set the provider accordingly in `goexpvar` and `inventories` and
    return
