# Molecule AMIs

Prerequisites:

* [Ansible](https://www.ansible.com/)
* [Packer](https://www.packer.io)

## Windows AMI

To prepare the Windows AMI that can be used for molecule tests you can run:

    make create-win-ami

It will create an administrator user that will be used by ansible, disable security checks and install needed test packages.

## Receiver AMI

To prepare the Receiver AMI that can be used for molecule tests you can run:

    make create-receiver-ami

It will create an Ubuntu based images enabling docker, docker-compose, terraform and kubectl and install needed test packages.

### Use the AMI

Once the creation is done an AMI will be provided, for example:

```
...
==> Builds finished. The artifacts of successful builds are:
--> amazon-ebs: AMIs were created:
eu-west-1: ami-0f39e4434caa6abaa
```

now you can use `ami-0f39e4434caa6abaa` for your molecule tests.

#### Reading materials

* https://www.packer.io/docs/provisioners/ansible.html
* https://github.com/hashicorp/packer/tree/master/examples/ansible/connection-plugin
