# OpenShift cluster setup

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Creating the Cluster](#creating-the-cluster)
- [Installing OpenShift](#installing-openshift)
- [Accessing and Managing OpenShift](#accessing-and-managing-openshift)
	- [OpenShift Web Console](#openshift-web-console)
	- [The Master Node](#the-master-node)
	- [The OpenShift Client](#the-openshift-client)
- [Connecting to the Docker Registry](#connecting-to-the-docker-registry)
- [Additional Configuration](#additional-configuration)
- [Choosing the OpenShift Version](#choosing-the-openshift-version)
- [Destroying the Cluster](#destroying-the-cluster)
- [Troubleshooting](#troubleshooting)
- [Developer Guide](#developer-guide)
	- [Linting](#linting)

## Overview

Terraform is used to create infrastructure based on EC2 instances as shown:

![Network Diagram](./aws-ec2/tf-cluster/docs/network-diagram.png)

Once the infrastructure is set up an inventory of the system is dynamically
created, which is used to install the OpenShift Origin platform on the hosts.

## Prerequisites

You need:

1. [Terraform](https://www.terraform.io/intro/getting-started/install.html) - `brew update && brew install terraform`
2. An AWS account, configured with the cli locally -
    ```
    if [[ "$unamestr" == 'Linux' ]]; then
            dnf install -y awscli || yum install -y awscli
    elif [[ "$unamestr" == 'FreeBSD' ]]; then
            brew install -y awscli
    fi
    ```
3. Export the follwing environment variables -
    ```
    export AWS_ACCESS_KEY_ID=
    export AWS_SECRET_ACCESS_KEY=
    export TF_VAR_AWS_SECRET_ACCESS_KEY=
    export TF_VAR_AWS_ACCESS_KEY_ID=
    export TF_VAR_AWS_REGION=eu-west-1
    export TF_VAR_CLUSTER_NAME=
    ```
4. Generate a Keypair for SSH 
    ```
    ssh-keygen -f okd_rsa
    ```

## Creating the Cluster

Create the infrastructure first:

```bash
# Make sure ssh agent is on, you'll need it later.
eval `ssh-agent -s`

# Create the infrastructure.
make infrastructure
```


That's it! The infrastructure is ready and you can install OpenShift. Leave about five minutes for everything to start up fully.

## Installing OpenShift

To install OpenShift on the cluster, just run:

```bash
make openshift
```

You will be asked to accept the host key of the bastion server (this is so that the install script can be copied onto the cluster and run), just type `yes` and hit enter to continue.

It can take up to 30 minutes to deploy. If this fails with an `ansible` not found error, just run it again.

Once the setup is complete, just run:

```bash
make browse-openshift
```

To open a browser to admin console, use the following credentials to login:

```
Username: admin
Password: 123
```

## Accessing and Managing OpenShift

There are a few ways to access and manage the OpenShift Cluster.

### OpenShift Web Console

You can log into the OpenShift console by hitting the console webpage:

```bash
make browse-openshift

# the above is really just an alias for this!
open $(terraform output master-url)
```

The url will be something like `https://a.b.c.d.xip.io:8443`.

### The Master Node

The master node has the OpenShift client installed and is authenticated as a cluster administrator. If you SSH onto the master node via the bastion, then you can use the OpenShift client and have full access to all projects:

```
$ make ssh-master
$ oc get pods
NAME                       READY     STATUS    RESTARTS   AGE
docker-registry-1-d9734    1/1       Running   0          2h
registry-console-1-cm8zw   1/1       Running   0          2h
router-1-stq3d             1/1       Running   0          2h
```

Notice that the `default` project is in use and the core infrastructure components (router etc) are available.

You can also use the `oadm` tool to perform administrative operations:

```
$ oadm new-project test
Created project test
```

Note: this is not recommended for production setup 
Cluster admin user is enabled only using the master node; to enable the normal admin user to behave as a cluster admin execute this on the master:

```
$ oc adm policy add-cluster-role-to-user cluster-admin admin
```

after that re-login as admin on your machine:
```
$ oc login -u admin
```

### The OpenShift Client

From the OpenShift Web Console 'about' page, you can install the `oc` client, which gives command-line access. Once the client is installed, you can login and administer the cluster via your local machine's shell:

```bash
oc login $(terraform output master-url)
```

Note that you won't be able to run OpenShift administrative commands. To administer, you'll need to SSH onto the master node. Use the same credentials (`admin/123`) when logging through the commandline.

## Connecting to the Docker Registry

The OpenShift cluster contains a Docker Registry by default. You can connect to the Docker Registry, to push and pull images directly, by following the steps below.

First, make sure you are connected to the cluster with [The OpenShift Client](#The-OpenShift-Client):

```bash
oc login $(terraform output master-url)
```

Now check the address of the Docker Registry. Your Docker Registry url is just your master url with `docker-registry-default.` at the beginning:

```
% echo $(terraform output master-url)
https://54.85.76.73.xip.io:8443
```

In the example above, my registry url is `https://docker-registry-default.54.85.76.73.xip.io`. You can also get this url by running `oc get routes -n default` on the master node.

You will need to add this registry to the list of untrusted registries. The documentation for how to do this here https://docs.docker.com/registry/insecure/. On a Mac, the easiest way to do this is open the Docker Preferences, go to 'Daemon' and add the address to the list of insecure regsitries:

![Docker Insecure Registries Screenshot](./aws-ec2/tf-cluster/docs/insecure-registry.png)

Finally you can log in. Your Docker Registry username is your OpenShift username (`admin` by default) and your password is your short-lived OpenShift login token, which you can get with `oc whoami -t`:

```
% docker login docker-registry-default.54.85.76.73.xip.io -u admin -p `oc whoami -t`
Login Succeeded
```

You are now logged into the registry. You can also use the registry web interface, which in the example above is at: https://registry-console-default.54.85.76.73.xip.io

![Atomic Registry Screenshot](./aws-ec2/tf-cluster/docs/atomic-registry.png)

## Persistent Volumes

The cluster is set up with support for dynamic provisioning of AWS EBS volumes. This means that persistent volumes are supported. By default, when a user creates a PVC, an EBS volume will automatically be set up to fulfil the claim.

More details are available at:

- https://blog.openshift.com/using-dynamic-provisioning-and-storageclasses/
- https://docs.openshift.org/latest/install_config/persistent_storage/persistent_storage_aws.html

No additional should be required for the operator to set up the cluster.

Note that dynamically provisioned EBS volumes will not be destroyed when running `terrform destroy`. The will have to be destroyed manuallly when bringing down the cluster.


## Additional Configuration

The easiest way to configure is to change the settings in the [./inventory.template.cfg](./aws-ec2/tf-cluster/inventory.template.cfg) file, based on settings in the [OpenShift Origin - Advanced Installation](https://docs.openshift.org/latest/install_config/install/advanced_install.html) guide.

When you run `make openshift`, all that happens is the `inventory.template.cfg` is turned copied to `inventory.cfg`, with the correct IP addresses loaded from terraform for each node. Then the inventory is copied to the master and the setup script runs. You can see the details in the [`makefile`](./aws-ec2/tf-cluster/makefile).

## Choosing the OpenShift Version

Currently, OKD 3.11 is installed.

To change the version, you can attempt to update the version identifier in this line of the [`./aws-ec2/tf-cluster/install-from-bastion.sh`](./aws-ec2/tf-cluster/install-from-bastion.sh) script:

```bash
git clone -b release-3.11 https://github.com/openshift/openshift-ansible
```

However, this may not work if the version you change to requires a different setup. To allow people to install earlier versions, stable branches are available. Available versions are listed [here](https://github.com/openshift/openshift-ansible#getting-the-correct-version).


| Version | Status              | Branch                                                                                         |
|---------|---------------------|------------------------------------------------------------------------------------------------|
| 3.11    | Tested successfull | [`release/okd-3.11`](https://github.com/dwmkerr/terraform-aws-openshift/tree/release/okd-3.11) |
| 3.10    | Tested successfully | [`release/okd-3.10`](https://github.com/dwmkerr/terraform-aws-openshift/tree/release/okd-3.10) |
| 3.9     | Tested successfully | [`release/ocp-3.9`](https://github.com/dwmkerr/terraform-aws-openshift/tree/release/ocp-3.9)   |
| 3.8     | Untested |                                                                                                |
| 3.7     | Untested |                                                                                                |
| 3.6     | Tested successfully | [`release/openshift-3.6`](tree/release/openshift-3.6) |
| 3.5     | Tested successfully | [`release/openshift-3.5`](tree/release/openshift-3.5) |


## Destroying the Cluster

Bring everything down with:

```
terraform destroy
```

Resources which are dynamically provisioned by Kubernetes will not automatically be destroyed. This means that if you want to clean up the entire cluster, you must manually delete all of the EBS Volumes which have been provisioned to serve Persistent Volume Claims.

## Makefile Commands

There are some commands in the `makefile` which make common operations a little easier:

| Command                 | Description                                     |
|-------------------------|-------------------------------------------------|
| `make infrastructure`   | Runs the terraform commands to build the infra. |
| `make openshift`        | Installs OpenShift on the infrastructure.       |
| `make browse-openshift` | Opens the OpenShift console in the browser.     |
| `make ssh-bastion`      | SSH to the bastion node.                        |
| `make ssh-master`       | SSH to the master node.                         |
| `make ssh-node1`        | SSH to node 1.                                  |
| `make ssh-node2`        | SSH to node 2.                                  |
| `make lint`             | Lints the terraform code.                       |



## Troubleshooting

**Image pull back off, Failed to pull image, unsupported schema version 2**

Ugh, stupid OpenShift docker version vs registry version issue. There's a workaround. First, ssh onto the master:

```
$ ssh -A ec2-user@$(terraform output bastion-public_ip)

$ ssh master.openshift.local
```

Now elevate priviledges, enable v2 of of the registry schema and restart:

```bash
sudo su
oc set env dc/docker-registry -n default REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_ACCEPTSCHEMA2=true
systemctl restart origin-master.service
```

You should now be able to deploy. [More info here](https://github.com/dwmkerr/docs/blob/master/openshift.md#failed-to-pull-image-unsupported-schema-version-2).

**OpenShift Setup Issues**

```
TASK [openshift_manage_node : Wait for Node Registration] **********************
FAILED - RETRYING: Wait for Node Registration (50 retries left).

fatal: [node2.openshift.local -> master.openshift.local]: FAILED! => {"attempts": 50, "changed": false, "failed": true, "results": {"cmd": "/bin/oc get node node2.openshift.local -o json -n default", "results": [{}], "returncode": 0, "stderr": "Error from server (NotFound): nodes \"node2.openshift.local\" not found\n", "stdout": ""}, "state": "list"}
        to retry, use: --limit @/home/ec2-user/openshift-ansible/playbooks/byo/config.retry
```

This issue appears to be due to a bug in the kubernetes / aws cloud provider configuration, which is documented here:

https://github.com/dwmkerr/terraform-aws-openshift/issues/40

At this stage if the AWS generated hostnames for OpenShift nodes are specified in the inventory, then this problem should disappear. If internal DNS names are used (e.g. node1.openshift.internal) then this issue will occur.

**Unable to restart service origin-master-api**

```
Failure summary:


  1. Hosts:    ip-10-0-1-129.ec2.internal
     Play:     Configure masters
     Task:     restart master api
     Message:  Unable to restart service origin-master-api: Job for origin-master-api.service failed because the control process exited with error code. See "systemctl status origin-master-api.service" and "journalctl -xe" for details.
```



### Linting

[`tflint`](https://github.com/wata727/tflint) is used to lint the code on the CI server. You can lint the code locally with:

```bash
make lint
```

