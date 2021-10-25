# Molecule - Localinstall

### Summary
- Supports Multiple Stages: ✅
- Common File Sharing: ✅
- Docker Compose Base File Usage: ✅
- Recreates the EC2 instances in the seconds stage if not present: ✅
- Share Vars between Run and Create playbooks: ✅
- Unit Testing: ✅
- Shared Group Vars: ✅

### Commands
- cd test
- ./molecule3.sh localinstall create
- ./molecule3.sh localinstall prepare
- (optional) ./molecule3.sh localinstall login
- ./molecule3.sh localinstall test

## Folder Structure

- `.cache`
  - Generated Folder
  - This is basically a copy of your local molecule cache folder, Acts the same as Artifacts


- `group_vars`
  - Shared variables between the various playbooks


- `playbook/setup`
  - The scripts inside this folder is used with the following commands
    - `./molecule3.sh localinstall create`
    - `./molecule3.sh localinstall destroy`
  - The `playbook/run` will also use these files if there is no EC2 present when the `./molecule3.sh compose prepare` step is executed


- `playbook/run`
  - The scripts inside this folder is used with the following commands
    - `./molecule3.sh localinstall prepare`
    - `./molecule3.sh localinstall test`


- `tests`
  - Contains all the unit tests ran with the following commands
    - `./molecule3.sh localinstall test`

## Execution Order

1) Execute `./molecule3.sh localinstall create`
   1) This will attempt to run `molecule` with the `provisioner.setup.yml` provisioner
   2) Inside this provision the following seq of steps will execute
      1) destroy: `playbook/setup/destroy.yml`
         1) Cleans up all the existing EC2 instances and Keypairs
      2) create: `playbook/setup/create.yml`
         1) Creates the required `Security Group`
         2) Creates the required `EC2 Instance`
         3) Creates the required `Key Pairs`
      3) prepare: `playbook/setup/prepare.yml`
         1) Run any required `installs and updates`
         2) `Login` to the required portals IE docker
         3) `Copies` the local files and folders over that's required for execution IE docker


2) Execute `./molecule3.sh localinstall prepare`
   1) This will attempt to run `molecule` with the `provisioner.run.yml` provisioner
   2) Inside this provision the following seq of steps will execute
      1) create: `playbook/run/create.yml`
         1) **IMPORTANT** - We first do a search for a available EC2 machine with this file `_shared/aws/determine-create-state.yml`
         2) If a relevant EC2 machine is found then we set the `block_ec2_creation` variable, If one is not found then the `block_ec2_creation` will not exist
         3) If the `block_ec2_creation` does not exist then we first execute the prev step `playbook/setup/create.yml` to create the machine
         4) We then setup the basic requirements for the prepare stage to connect to the EC2 SSH Machine
      2) prepare: `playbook/run/prepare.yml`
         1) If the `block_ec2_creation` does not exist then we first execute the prev step `playbook/setup/prepare.yml` to prepare the newly created machine
         2) The execution steps is then ran IE 'docker localinstall' etc.


3) Execute `./molecule3.sh localinstall login`
   1) Optional step to login into the EC2 machine


4) Execute `./molecule3.sh localinstall test`
   1) Execute the unit test inside the `tests` folder


5) Execute `./molecule3.sh localinstall destroy`
   1) This will cleanup the EC2 and Keypair
