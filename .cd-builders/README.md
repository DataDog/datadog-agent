# CD Builders

Here you can find all our builders that are as gitlab runner.

## Linux

The `runner-circle` is the base docker image for debian and rpm builder images.

## Windows

Under `windows` we have the runner that can be created with:

    $ ./windows/runner-gitlab/ansible_gitlab_runner.sh

Once the EC2 has been deployed all tools needed for building can be provisioned:

    $ cd windows/builder
    $ cp hosts.example hosts
    $ ... replace EC2 IP in hosts ...
    $ make provision
