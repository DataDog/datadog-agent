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

Make sure if you change the AWS_REGION to find the correct vpc subnet and replace it in `default/molecule.yml`.

Now execute `./molecule.sh`, this will show you the help.

To run a single ansible command use:

    $ source p-env/bin/activate
    $ ansible agent-ubuntu -i /tmp/molecule/molecule-role/default/ansible_inventory.yml -m debug -a msg="{{ ansible_facts }}"
    
    or on MacOS X:
    $ ansible agent-ubuntu -i /var/folders/.../molecule/molecule-role/default/ansible_inventory.yml -m debug -a msg="{{ ansible_facts }}"


## Windows image for molecule

Under `./molecule-role/win-image-refresh` there is a terraform script that can be used to bake an instance of Windows to be used in our molecule test

    $ cd molecule-role/win-image-refresh
    $ terraform init
    $ terraform plan -o win.plan
    $ terraform apply -f win.plan
