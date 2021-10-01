FROM docker.io/stackstate/stackstate-agent-runner-circle:centos6_latest_v7

#workaround

RUN sed -i -e 's/^mirrorlist/#mirrorlist/g' -e 's/^#baseurl=http:\/\/mirror.centos.org\/centos\/$releasever\//baseurl=http:\/\/vault.centos.org\/6.10\//g' /etc/yum.repos.d/CentOS-Base.repo && \
    sed -i -e 's/^mirrorlist/#mirrorlist/g' -e 's/^#baseurl=http:\/\/mirror.centos.org\/centos\/$releasever\//baseurl=http:\/\/vault.centos.org\/6.10\//g' /etc/yum.repos.d/CentOS-fasttrack.repo && \
    sed -i -e 's/^mirrorlist/#mirrorlist/g' -e 's/^#baseurl=http:\/\/mirror.centos.org\/centos\/$releasever\//baseurl=http:\/\/vault.centos.org\/6.10\//g' /etc/yum.repos.d/CentOS-SCLo-scl-rh.repo && \
    rm /etc/yum.repos.d/CentOS-SCLo-scl.repo && \
    rm /etc/yum.repos.d/CentOS-SCLo-scl-rh.repo

# force update root certificates
RUN yum -y update ca-certificates

# validate we are able to manage with https:// cert
RUN curl https://curl.se/ca/cacert.pem

# /workaround

RUN pip install awscli boto3 sa-rpm-s3

RUN yum -y install net-snmp-devel python-deltarpm yum-utils libtool rpm-sign createrepo

ADD rpmmacros /root/.rpmmacros

SHELL ["/bin/bash", "-c"]
