#!/usr/bin/env bash

# This script template is expected to be populated during the setup of a
# OpenShift  node. It runs on host startup.

# Log everything we do.
set -x
exec > /var/log/user-data.log 2>&1

mkdir -p /etc/aws/
cat > /etc/aws/aws.conf <<- EOF
[Global]
Zone = ${availability_zone}
EOF

# Create initial logs config.
cat > ./awslogs.conf <<- EOF
[general]
state_file = /var/awslogs/state/agent-state

[/var/log/messages]
log_stream_name = openshift-node-{instance_id}
log_group_name = /var/log/messages
file = /var/log/messages
datetime_format = %b %d %H:%M:%S
buffer_duration = 5000
initial_position = start_of_file

[/var/log/user-data.log]
log_stream_name = openshift-node-{instance_id}
log_group_name = /var/log/user-data.log
file = /var/log/user-data.log
EOF

# Download and run the AWS logs agent.
curl https://s3.amazonaws.com/aws-cloudwatch/downloads/latest/awslogs-agent-setup.py -O
python ./awslogs-agent-setup.py --non-interactive --region ${region} -c ./awslogs.conf

# Start the awslogs service, also start on reboot.
# Note: Errors go to /var/log/awslogs.log
service awslogs start
chkconfig awslogs on

# OpenShift setup
# See: https://docs.openshift.org/latest/install_config/install/host_preparation.html

# Install packages required to setup OpenShift.
yum install -y wget git net-tools bind-utils iptables-services bridge-utils bash-completion
yum update -y

# Note: The step below is not in the official docs, I needed it to install
# Docker. If anyone finds out why, I'd love to know.
# See: https://forums.aws.amazon.com/thread.jspa?messageID=574126
yum-config-manager --enable rhui-REGION-rhel-server-extras

# Docker setup. Check the version with `docker version`, should be 1.12.
yum install -y docker

# Configure the Docker storage back end to prepare and use our EBS block device.
# https://docs.openshift.org/latest/install_config/install/host_preparation.html#configuring-docker-storage
# Why xvdf? See:
# http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ebs-using-volumes.html
cat <<EOF > /etc/sysconfig/docker-storage-setup
DEVS=/dev/xvdf
VG=docker-vg
EOF
docker-storage-setup

# Restart docker and go to clean state as required by docker-storage-setup.
systemctl stop docker
rm -rf /var/lib/docker/*
systemctl restart docker

# Allow the ec2-user to sudo without a tty, which is required when we run post
# install scripts on the server.
echo Defaults:ec2-user \!requiretty >> /etc/sudoers
