#!/bin/sh

terraform output Makefile > Makefile

terraform output hosts > hosts

ansible-playbook -i hosts provisioner/playbook_agent.yml

