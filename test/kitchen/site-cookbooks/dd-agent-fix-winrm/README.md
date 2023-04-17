# dd-agent-fix-winrm cookbook

Cookbook that updates the WinRM maximum memory used on old Windows Server versions.

## Why?

In old Windows Server versions, doing:

```
winrm set winrm/config/winrs '@{MaxMemoryPerShellMB="<new amount>"}'
```

is not enough to increase the maximum memory usable by the WinRM connection. You also need to run:

```
Set-Item WSMan:\localhost\Plugin\Microsoft.PowerShell\Quotas\MaxMemoryPerShellMB <new amount>
```

and then restart the winrm service, or reboot the host.

This cookbook performs the `Set-Item` operation (only if its value is not the one we need, default `4096`), and then reboots the host.
To use the host, you need to allow Chef to automatically retry converging (`max_retries` > 1 in the `provisioner` config), and also configure the `transport` settings to retry the WinRM connection enough times to let the server get back up.
