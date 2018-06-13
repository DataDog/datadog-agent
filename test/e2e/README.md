# End to End testing

# Upgrade - bump

The current end to end testing pipeline relies on
* pupernetes
* argo
* kinvolk flatcar

## Bump hyperkube version

Read the command lines docs of [pupernetes](https://github.com/DataDog/pupernetes/tree/master/docs)

In the Ignition *systemd.units* list, find the `pupernetes.service` one.

In its content, the systemd unit has a `[service]` directive where there is a single `ExecStart=` field: you need to add / edit the `--hyperkube-version` flag in the command line.

The value of this flag must be a valid release such as `1.10.1`: `--hyperkube-version=1.10.1`

Concrete example, I want to bump the hyperkube version from 1.9.3 to 1.10.1:
```json
{
  "systemd": {
    "units": [
      {
        "enabled": true,
        "name": "pupernetes.service",
        "contents": "[Unit]\nDescription=Run pupernetes\nRequires=setup-pupernetes.service docker.service\nAfter=setup-pupernetes.service docker.service\n\n[Service]\nEnvironment=SUDO_USER=core\nExecStart=/opt/bin/pupernetes daemon run /opt/sandbox --hyperkube-version 1.9.3 --kubectl-link /opt/bin/kubectl -v 5 --timeout 48h\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=multi-user.target\n"
      }
    ]
  }
}
```

Become:
```json
{
  "systemd": {
    "units": [
      {
        "enabled": true,
        "name": "pupernetes.service",
        "contents": "[Unit]\nDescription=Run pupernetes\nRequires=setup-pupernetes.service docker.service\nAfter=setup-pupernetes.service docker.service\n\n[Service]\nEnvironment=SUDO_USER=core\nExecStart=/opt/bin/pupernetes daemon run /opt/sandbox --hyperkube-version 1.10.1 --kubectl-link /opt/bin/kubectl -v 5 --timeout 48h\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=multi-user.target\n"
      }
    ]
  }
}
```


## Bump pupernetes - bump the version of p8s

Have a look at [pupernetes](https://github.com/DataDog/pupernetes/tree/master/environments/container-linux).

Generate the `ignition.json` with the `.py` and the `.yaml`:
```bash
${GOPATH}/src/github.com/DataDog/pupernetes/environments/container-linux/ignition.py < ${GOPATH}/src/github.com/DataDog/pupernetes/environments/container-linux/ignition.yaml | jq .
```

Upgrade the relevant sections in [the ignition script](./scripts/setup-instance/01-ignition.sh).

The ignition script must insert the *core* user with their ssh public key:
```json
{
"passwd": {
    "users": [
      {
        "sshAuthorizedKeys": [
          "${SSH_RSA}"
        ],
        "name": "core"
      }
    ]
  }
}
```

If needed, use the [ignition-linter](https://coreos.com/validate/) to validate any changes.

## Bump argo

* change the binary version in [the argo setup script](./scripts/run-instance/21-argo-setup.sh)
* the content of [the sha512sum](./scripts/run-instance/argo.sha512sum)

## Bump CoreOS Container Linux - Kinvolk Flatcar

* change the value of the AMIs:
    * [dev](./scripts/setup-instance/00-entrypoint-dev.sh)
    * [gitlab](./scripts/setup-instance/00-entrypoint-gitlab.sh)


Select any HVM AMIs from:
* [kinvolk Flatcar](https://alpha.release.flatcar-linux.net/amd64-usr/)
* CoreOS Container linux
