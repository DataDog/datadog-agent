Agent Molecule tests
--------------------

Those are integration tests that spawn new VMs in AWS and do the following:

* install the agents from the debian/rpm repositories
* run a docker compose setup of the StackState receiver, correlate and topic API
* verify assertion on the target VMs

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

Test are organized by scenarios, they are directories located under `molecule-role/molecule` and all molecule commands need to target a scenario, like:

    ./molecule.sh test -s <scenario>
    ./molecule.sh create -s <scenario>
    ./molecule.sh verify -s <scenario>

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

