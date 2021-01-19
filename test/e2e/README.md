# End to End testing

# ToC
- [How it works](#how-it-works)
  * [Setup instance](#setup-instance)
  * [Run instance](#run-instance)
  * [Command line](#command-line)
    * [AWS development](#aws-development)
    * [Locally](#locally)
- [Argo workflow](#argo-workflow)
  * [argo assertion](#argo-assertion)
  * [argo container](#argo-container)
- [Upgrade](#upgrade---bump)
  * [Hyperkube](#bump-hyperkube-version)
  * [Pupernetes](#bump-pupernetes---bump-the-version-of-p8s)
  * [Argo](#bump-argo)
  * [Flatcar - Container Linux](#bump-coreos-container-linux---kinvolk-flatcar)

# How it works

There are 3 main directories:
- [argo-workflows](./argo-workflows)
    Specification of the end to end testing

- [containers](./containers) 
    Custom container images needed within the workflows

- [scripts](./scripts)
    - [`setup-instance`](./scripts/setup-instance)
      Entrypoint and scripts dedicated for environments (locally, AWS dev, AWS gitlab)
    - [`run-instance`](./scripts/run-instance)
      Scripts executed in the argo-machine (locally, AWS instance)

## `setup-instance`

<img src="docs/setup-instance.svg" width="350">

## `run-instance`

You need [pupernetes](https://github.com/DataDog/pupernetes):
```bash
$ curl -LO https://github.com/DataDog/pupernetes/releases/download/v${VERSION}/pupernetes
$ chmod +x ./pupernetes  
```

<img src="docs/run-instance.svg" width="100">

## Command line

### AWS development

```bash
$ cd ${GOPATH}/src/github.com/DataDog/datadog-agent 
$ aws-vault exec ${DEV} -- inv -e e2e-tests -t dev --image datadog/agent-dev:master
$ echo $?
```

### Locally

```bash
$ sudo ./pupernetes daemon run /opt/sandbox --job-type systemd 
$ cd ${GOPATH}/src/github.com/DataDog/datadog-agent 
$ inv -e e2e-tests -t local --image datadog/agent-dev:master
$ echo $?
```

# Argo workflow

The argo documentation is available [here](https://applatix.com/open-source/argo/docs/examples.html), there are a lot of examples [here](https://github.com/argoproj/argo/tree/master/examples) too.

## Argo assertion

To assert something in an argo workflow, you need to create a mongodb query:
```yaml
name: find-kubernetes-state-deployments
activeDeadlineSeconds: 200
script:
  image: mongo:3.6.3
  command: [mongo, "fake-datadog.default.svc.cluster.local/datadog"]
  source: |
    while (1) {
      var nb = db.series.find({
      metric: "kubernetes_state.deployment.replicas_available", 
      tags: {$all: ["namespace:default", "deployment:fake-datadog"] }, 
      "points.0.1": { $eq: 1} });      
      print("find: " + nb)
      if (nb != 0) {
        break;
      }
      prevNb = nb;
      sleep(2000);
    }    
```

This is an infinite loop with a timeout set by `activeDeadlineSeconds: 200`.
The source is EOF to the command, equivalent to:
```bash
mongo "fake-datadog.default.svc.cluster.local/datadog" << EOF
while (1)
[...]
EOF
```

Try to maximise the usage of MongoDB query system without rewriting too much logic in JavaScript.

See some examples [here](./containers/fake_datadog/README.md#find)

To discover more MongoDB capabilities:
- [find](https://docs.mongodb.com/manual/tutorial/query-documents/)
- [aggregation](https://docs.mongodb.com/manual/aggregation/)

## Argo container

If you need to add a non existing public container in the workflow, create it in the [container directory](./containers).

But, keep in mind this become an additional piece of software to maintain.



# Upgrade - bump

This section helps you to upgrade any part of the end to end testing.

The current end to end testing pipeline relies on:
* [Pupernetes](https://github.com/DataDog/pupernetes)
* [Argo](https://github.com/argoproj/argo)
* [Kinvolk Flatcar](https://www.flatcar-linux.org)

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

* Change the binary version in [the argo setup script](./scripts/run-instance/21-argo-setup.sh)
* The content of [the sha512sum](./scripts/run-instance/argo.sha512sum)

## Bump CoreOS Container Linux - Kinvolk Flatcar

* Change the value of the AMIs:
    * [dev](./scripts/setup-instance/00-entrypoint-dev.sh)
    * [gitlab](./scripts/setup-instance/00-entrypoint-gitlab.sh)


Select any HVM AMIs from:
* [Kinvolk Flatcar](https://alpha.release.flatcar-linux.net/amd64-usr/)
* CoreOS Container linux
