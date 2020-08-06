FROM docker.io/stackstate/stackstate-agent-runner-circle:centos6_latest_v7

RUN pip install awscli boto3 sa-rpm-s3

RUN yum -y install net-snmp-devel python-deltarpm yum-utils libtool rpm-sign createrepo

ADD rpmmacros /root/.rpmmacros

SHELL ["/bin/bash", "-c"]
