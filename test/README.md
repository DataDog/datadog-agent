Agent Molecule tests
--------------------

Those are integration tests that spawn new VMs in AWS and do the following:

* install the agent from the debian repository
* run a docker compose setup of the StackState receiver
* verify assertion on the target VMs

### Run

Prerequisites:

* export AWS_ACCESS_KEY_ID=
* export AWS_SECRET_ACCESS_KEY=
* export AWS_REGION=eu-west-1

Make sure if you change the AWS_REGION to find the correct vpc subnet and replace it in `default/molecule.yml`.

Now execute `./molecule.sh`, this will show you the help.
