Agent Molecule tests
--------------------

Those are integration tests that spawn new VMs in AWS and do the following:

* install the agents from the debian/rpm repositories
* run a docker compose setup of the StackState receiver, correlate and topic API
* verify assertion on the target VMs

## ** Important Notes ** (Auto Cleanup, and unreachable machines)
Gitlab CI Process:
- Molecule instances spun up won't run for longer than 2 hours and 30 minutes. After the max time, a script will clean those instances to prevent EC2 costs from racking up from zombie instances
- Only one instance per branch is allowed. What that means is if you push up another commit, the prepare stage will destroy the ec2 machine from your previous builds. If you get a message that says x-x-x-branch-name-lock, that means you have a prev branch still running and need to kill that branch to release the lock to allow your new branch to continue. This prevents a lot of useless EC2 instances from running and building up costs (We did include interrupt but GitLab seems to be broken on interrupts atm)
- If a step in the "Cleanup" stage ran to destroy a molecule machine, or your acceptance step complains about "Unable to reach the ssh machine", then your molecule instance might have been destroyed or cleaned up. To recreate it, rerun the appropriate "Prepare" stage step to spin the machine back up

Supported Commit Message Functionality (Target builds and reduce EC2 costs):

** Default: Note these do not have to be defined, by default everything will be included except secrets and localinstall which falls on master and tags **

The following will run a single molecule pipeline and ignore the rest
this will reduce ec2 costs, possible wait times and clutter
- "<commit message> [molecule-compose]"
- "<commit message> [molecule-integrations]"
- "<commit message> [molecule-secrets]"
- "<commit message> [molecule-localinstall]"
- "<commit message> [molecule-kubernetes]"
- "<commit message> [molecule-swarm]"
- "<commit message> [molecule-vms]"

You can also reduce ec2 costs and clutter by defining the py version you want to build
- "<commit message> [py2]"
- "<commit message> [py3]"

This would be ideal to reduce the pipeline build to only what's required
You can combine tags from the top to stack filters for example:
- "<commit message> [py2][molecule-compose]"
- "<commit message> [py3][molecule-secrets][molecule-vms]"
-
## Run

Prerequisites:

* export AWS_ACCESS_KEY_ID=
* export AWS_SECRET_ACCESS_KEY=
* export AWS_REGION=eu-west-1

Make sure if you change the AWS_REGION to find the correct vpc subnet and replace it in `molecule.yml`.

Now execute `./molecule.sh`, this will show you the help.

### When using the new security setup on local machines:

* export AWS_PROFILE=stackstate-sandbox

The AWS credentials will be picked up from your ~/.aws/credentials file

### Test

The molecule script executions has a few requirements:
- Base config path
   - **setup**: Used to spin up the instance and run the first prepare step
   - **run**: Executes the primary prepare step which includes the docker compose etc.
- The molecule action to take
- The molecule scenario name

https://miro.com/app/board/o9J_lzUC0FM=/

Test are organized by scenarios, they are directories located under `molecule-role/molecule` and all molecule commands need to target a scenario, like:

Example Charts of how this is used on the Gitlab CI: https://miro.com/app/board/o9J_lzUC0FM=/

WARNING: If you create any instance from you local machine please delete it seeing that Lambda does not clean dev instances thus the EC2 costs will increase the longer that instances stays up

    First step is to create the EC2 machine
    -  ./molecule3.sh <scenario> create

    After that we copy over all the required files, install updates and deps, cache images etc.
    - ./molecule3.sh <scenario> prepare

    Now you can either login into your machine with SSH or
    -  ./molecule3.sh <scenario> login

    Run the docker-compose and the unit tests (Note that everytime you run this a docker-compose cleanup is also ran to cleanup your prev run)
    -  ./molecule3.sh <scenario> test

    Destroy the EC2 machine and Keypair you created
    -  ./molecule3.sh <scenario> destroy

Available scenarios
- compose
- integrations
- kubernetes
- localinstall
- secrets
- swarm
- vms

### Troubleshooting

To run a single ansible command use you can use the scenario inventory:

    $ source p-env/bin/activate
    $ ansible agent-ubuntu -i /tmp/molecule/molecule-role/vms/inventory/ansible_inventory.yml -m setup

    or on MacOS X:
    $ ansible agent-ubuntu -i /var/folders/.../molecule/molecule-role/vms/inventory/ansible_inventory.yml -m setup


## Windows image for molecule

Under `./molecule-role/win-image-refresh` there is a terraform script that can be used to bake an instance of Windows to be used in our molecule test

    $ cd molecule-role/win-image-refresh
    $ terraform init
    $ terraform plan -o win.plan
    $ terraform apply -f win.plan

## Emulating pipeline molecule run locally

```sh

export MOLECULE_RUN_ID=${USER}_manual
export AGENT_CURRENT_BRANCH=`git rev-parse --abbrev-ref HEAD`
export quay_password=SPECIFY_ENCRYPTED_CHECK_UI
export quay_user=SPECIFY
export STACKSTATE_BRANCH=master
```

you can only converge environment, like

```sh
cd test/molecule-role
molecule converge -s vms
```

Important, do not leave dangling instances on a cloud after manual troubleshouting.

In compose scenario to get round of traces

```sh
curl -H Host:stackstate-books-app -s -o /dev/null -w \"%{http_code}\" http://localhost/stackstate-books-app/listbooks
```
getting intercepted payload from simulator

```sh
curl -o out.json http://localhost:7077/download
```


### Leaving instances up for troubleshouting
```sh
molecule test -s vm --destroy=never
```

### Escaping root device size

```yaml

- name: ami facts
  ec2_ami_info:
     image_ids: "{{ ami_id }}"
  register: ami_facts

- name: set current ami
  set_fact:
     ami: "{{ ami_facts.images | first }}"
```


and use ami.root_device_name


you can also get the same information from console, like

```sh
 ansible localhost -m ec2_ami_info -a "image_ids=ami-09ae46ee3ab46c423" | grep root_device
```


