FROM docker.io/stackstate/stackstate-agent-runner-circle:latest

RUN pip install awscli boto3 sa-rpm-s3
RUN apt-get install -y libkrb5-dev unixodbc-dev rpm python-deltarpm createrepo yum-utils

ADD rpmmacros /root/.rpmmacros
