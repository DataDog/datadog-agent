0. Configure windows ESXi box for cross-host copy pasting

VMOptions -> Advanced -> Edit parameters

set

```s

isolation.tools.copy.disable FALSE
isolation.tools.paste.disable FALSE
isolation.tools.setGUIOptions.enable TRUE

```



1. Prepare your winbox for ansible provisioning

On a escalated powershell

```ps1

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Set-ExecutionPolicy Bypass -Scope Process -Force; iex ((New-Object System.Net.WebClient).DownloadString('https://bit.ly/ansible_remoting'))

```

on the step you might consider to enable global confirmationssss

```ps1
ss
choco feature enable -n allowGlobalConfirmation
```

Depending on your OS, you might also face

https://support.microsoft.com/en-us/help/2842230/out-of-memory-error-on-a-computer-that-has-a-customized-maxmemorypersh

also you might face limit  https://github.com/ansible/ansible/issues/39327

```ps1
# 2048 4096 8192 this is the max mem in MB
Set-Item -Path WSMan:\localhost\Shell\MaxMemoryPerShellMB -Value 2048
Restart-Service -Name WinRM
```


2. Validate connectivity by invoking make check

```sh

make check 
ansible windows -i hosts -m win_ping
192.168.3.126 | SUCCESS => {
    "changed": false, 
    "ping": "pong"
}
```



Woodo magic

ridk install 1
bash -c "yes | pacman -R catgets libcatgets"
ridk install 2
ridk install 3

Improval points possibly

mklink /J %GOPATH%\src\github.com\StackVista\stackstate-agent %WIN_CI_PROJECT%

