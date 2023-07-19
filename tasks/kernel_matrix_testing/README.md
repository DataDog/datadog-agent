# Kernel Matrix Testing system

## Overview
The Kernel Matrix Testing system is a new approach for testing system-probe. It uses libvirt and qemu to launch pre-provisioned VMs over a range of distributions and kernel versions. These VMs are used for running the test suite of system-probe.  

Developers can check out this confluence page for more details about the system.   

This file will document the invoke tasks provided to easily manage the lifecycle of the VMs launched using this system.

The system works on the concept of `stacks`. A `stack` is a collection of VMs, both local and remote. A `stack` is given a unique name by the user. Convenience options for generating the name of the `stack` from the current branch is also provided. This allows the developers to couple `stacks` with their git workflow.
A `stack` may be:
- Created
- Configured
- Launched
- Paused
- Resumed
- Destroyed

> All subsequent commands are assumed to be executed from the root directory of the datadog-agent repository.


> MacOS users can only launch remote VMs at the moment. Local development will be available soon.

## Dependencies
### Linux
Review and run `tasks/kernel_matrix_testing/env-setup.sh`   
   

- **You will have to run the following manually**
Download [test-infra-definitions](https://github.com/DataDog/test-infra-definitions) repository.   
From within the repository execute the following commands:  
```bash
go mod download
export PULUMI_CONFIG_PASSPHRASE=dummy
pulumi --non-interactive plugin install
pulumi --non-interactive plugin ls
```

### MacOS
```bash
brew install pulumi/tap/pulumi
pulumi login --local
```

Download [test-infra-definitions](https://github.com/DataDog/test-infra-definitions) repository.   
From within the repository execute the following commands:  
```bash
go mod download
export PULUMI_CONFIG_PASSPHRASE=dummy
pulumi --non-interactive plugin install
pulumi --non-interactive plugin ls
```
> Local development environment coming soon...

## Getting started
A straightforward flow to setup a collections of VMs is as follows:

### Initializing the environment
This will download all the resources required to launch the VMs. This will not download the dependencies. See [above](#Dependencies) for that.   

> This step should be done only once.

```bash
inv -e kmt.init
```

> You may skip the downloading part if you are not setting up VMs locally. This is true for mac users for now! Linux users should not skip the download.
```bash
inv -e kmt.init --lite
```

### Create stack
```bash
inv -e kmt.create-stack --stack=demo-stack
```

### Configure stack
We will configure the stack to launch 
- Remote x86_64 machine with ubuntu-jammy, ubuntu-focal VMs.
- Remote arm64 machine with amazon linux 2 kernel 4.14, amazon linux 2 kernel 5.10 VMs.
- Amazon linux 2 kernel 5.15, amazon linux 2 kernel 5.4 VMs on local machine.
```bash
inv -e kmt.gen-config --vms=x86-jammy-distro,x86-focal-distro,arm64-amazon4.14-distro,arm64-amazon5.10-distro,local-amazon5.15-distro,local-amazon5.4-distro --stack=demo-stack
```

### Launch stack
This will bring up all the VMs previously configured.
```bash
# SSH key name for the key to use for launching remobe machine in sandbox
# The tasks will automatically look for the key in ~/.ssh
inv -e kmt.launch-stack --stack=demo-stack --ssh-key=<ssh-key>
```

### Connecting to VMs
```bash
inv -e kmt.stack --stack=demo-stack
```
This will print the IP addresses of all the VMs and the remote machines if any

To connect to the VM first ssh to the remote machine, if required.

Then connect to the VM as follows
```bash
ssh -i /home/kernel-version-testing/ddvm_rsa -o StrictHostKeyChecking=no root@<ip>
```

### Destroy stack
Tear down the stack
```bash
inv -e kmt.destroy-stack --stack=demo-stack
```

If you run into any problems while destroying the stack, repeat with the following:
```bash
inv -e kmt.destroy-stack --force
```

This will attempt to manually teardown all resources. Primarily we care about cleaning the local libvirt environment and destroy remote ec2 instances. This flag takes care of these steps.

## Tasks

### Initializing environment
```bash
inv -e kmt.init
```

If you only want to initialize the directory structure, you may do a 'lite' setup as follows:
```bash
inv -e kmt.init --lite
```

### Updating resources
In order to update the resources for launching VMs run:
```bash
inv -e kmt.update-resources
```

Updating will first destroy all running stacks, and then use checksums to decide which packages need to be updated from S3.   
If there is an error during update, original packages are restored from backup.


### Revert resources
During the update process, all packages are first backed-up, incase there is an error during the update.   
This backup may be resotred manually if there is a problem with the new resources.

```bash
inv -e kmt.revert-resources
```

Reverting will destroy all running stacks before restoring from backup.

### Creating a stack
A stack can be created as follows:
```bash
inv -e kmt.create-stack [--stack=<name>|--branch]
```
The developer needs to provide a name to associate with the `stack`, or specify the `--branch` argument to generate the name from the current branch.

### Listing possible VMs
Possible VMs can be listed with
```bash
inv -e kmt.ls
```

The arguments to this are:
- `--distro`: Only list distribution images.
- `--custom`: Only list custom kernels.
- No argument will list everything.

### Configuring the stack
Configuring the stack involves generating a configuration file which specifies the the VMs to launch. Henceforth, this file will be referred to as the `vmsets` file.

The `vmsets` file contains the list of one or more sets of VMs to launch. A set of VMs refers to a collection of VMs sharing some characteristic. The following is an exhaustive list of possible sets:

- Custom x86_64 kernels on remote x86_64 machine.
- Custom arm64 kernels on remote arm64 machine.
- Custom kernels on local machine with corresponding architecture.
- x86_64 distribution on remote x86_64 machine.
- arm64 distribution on remote arm64 machine.
- local distribution with architecutre corresponding to the local machine.

Sample VMSet file can be found [here](https://github.com/DataDog/test-infra-definitions/blob/f85e7eb2f003b6f9693c851549fbb7f3969b8ade/scenarios/aws/microVMs/sample-vm-config.json).


The file can be generated for a particular `stack` with the `gen-config` task.   
This task takes as parameters
- The stack to target specified with [--stack=<name>|--branch] 
- The list of VMS to generate specified by --vms=<list>. See [VMs list](#vms-list) for details on how to specify the list.
- Optional paramenter `--init-stack` to automatically initialize the stack. This can be specified to automatically run the creating stack step.
- Optional parameter `--new` to generate a fresh configuration file.

The file can be incrementally generated. This means a user may generate a vmset file. Launch it. Add more VMs. Launch them in the same stack.

#### Example 1
```bash
# Setup configuration file to launch jammy and focal locally. Initialize a new stack corresponding to the current branch
inv -e kmt.gen-config --branch --vms=jammy-local-distro,focal-local-distro --init-stack
# Launch this stack. Since we are launching local VMs, there is no need to specify ssh key.
inv -e kmt.launch-stack --branch
# Add amazon linux VMs to be launched locally
inv -e kmt.gen-config --branch --vms=amazon4.14-local-distro,distro-local-amazon5.15,distro-local-amazon5.10
# Launch the new VMs added. The previous VMs will keep running
inv -e kmt.launch-stack --branch
# Remove all VMs except amazon linux 2 4.14 locally
inv -e kmt.gen-config --branch --new --vms=amazon4.14-local-distro
# Apply this configuration
inv -e kmt.launch-stack --branch
```

#### Example 2
```bash
# Setup configuration file to launch ubuntu VMs on remote x86_64 and arm64 machines
inv -e kmt.gen-config --branch --vms=x86-ubuntu20-distro,distro-bionic-x86,distro-jammy-x86,distro-arm64-ubuntu22,arm64-ubuntu18-distro
# Name of the ssh key to use 
inv -e kmt.launch-stack --branch --ssh-key=<ssh-key-name>
# Add amazon linux
inv -e kmt.gen-config --branch --vms=x86-amazon5.4-disto,arm64-distro-amazon5.4
# Name of the ssh key to use 
inv -e kmt.launch-stack --branch --ssh-key=<ssh-key-name>
```

#### Example 3
```bash
# Configure custom kernels
inv -e kmt.gen-config --branch --vms=custom-5.4-local,custom-4.14-local,custom-5.7-arm64
# Launch stack
inv -e kmt.launch-stack --branch --ssh-key=<ssh-key-name>
```

### VMs List
The vms list is a comma separated list of vm entries. These are the VMs to launch in the stack.   
Each entry comprises of three elemets seperate by a `-` (dash).
1. Recipe. This is either `custom` or `distro`. `distro` is to be specified for distribution images and `custom` for custom kernels.
2. Arch. Architecture is either `x86_64`, `arm64` or `local`.
3. Version. This is either the distribution version for recipe `distro` or kernel version for recipe `custom`.

The vm entry is parsed in a fuzzy manner. Therefore each element can be inexact. Furthermore the order of the elements is not important either. The entry is only required to consist of `<recipe>-<arch|local>-<version>` in some order.

#### Example 1
 All of the below resolve to the entry [ubuntu-22, local, distro]
 - jammy-local-distro
 - distro-local-jammy
 - local-ubuntu22-distro

#### Example 2
All of the below resolve to [amazon linux 2 4.14, x86_64, distro]
- amazon4.14-x86-distro
- distro-x86_64-amazon4.14
- amzn_4.14-amd64-distro
- 4.14amazon-distro-x86

#### Example 3
All of the below resolve to [kernel 5.4, arm64, custom]
- custom-arm-5.4
- 5.4-arm64-custom
- custom-5.4-aarch64

### Launching the stack
If you are just launching local VMs you do not need to specify an ssh key
```bash
inv -e kmt.launch-stack --branch
```

If you are launching remote instances then the ssh key used to access the machine is required.   
Only the ssh key name is required. The program will automatically look in `~/.ssh/` directory for the key.
```bash
inv -e kmt.launch-stack --branch --ssh-key=<ssh-key-name>
```

If you are launching local VMs, you will be queried for you password. This is required since the program has to run some commands as root. However, we do not run the entire scenario with `sudo` to avoid broken permissions.

### List the stack
Prints information about the stack.
```bash
inv -e kmt.stack [--stack=<name>|--branch]
```

> At the moment this just prints the running VMs and their IP addresses. This information will be enriched in later versions of the tool.

### Pausing the stack
This is only relevant for VMs running in the local environment. This has no effect on VMs running on remote instances. Pausing the stack essentially stops the running VMs and frees their resources. However, the VM environment is left intact so that it may be brought up again.

### Resuming the stack
This resumes a previously paused stack. This is only applicable for VMs running locally.


### Syncing VMs
The recommended workflow is to develop on the local machine, push changes to the target VMs, and then build and test there.

To support this, a task for syncing the local datadog-agent repo with target VMs is provided.  

> We are working on providing tasks which will automatically sync with the target VMs, and build and run tests. However, this is a work in progress.

If syncing to VMs running on remote machine the ssh-key-name is required. For local VMs it is not required.
The VMs list has the same rules as listed for the task `gen-config`.
```bash
inv -e kmt.sync --branch --vms=local-amazon4.14-distro,jammy-local-distro,focal-arm-distro,amazon5.4-x86-distro --ssh-key=<ssh-key-name>
```

