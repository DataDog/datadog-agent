FROM docker.io/stackstate/stackstate-agent-runner-circle:centos6_latest_v7

#workaround

RUN sed -i -e 's/^mirrorlist/#mirrorlist/g' -e 's/^#baseurl=http:\/\/mirror.centos.org\/centos\/$releasever\//baseurl=http:\/\/vault.centos.org\/6.10\//g' /etc/yum.repos.d/CentOS-Base.repo && \
    sed -i -e 's/^mirrorlist/#mirrorlist/g' -e 's/^#baseurl=http:\/\/mirror.centos.org\/centos\/$releasever\//baseurl=http:\/\/vault.centos.org\/6.10\//g' /etc/yum.repos.d/CentOS-fasttrack.repo && \
    sed -i -e 's/^mirrorlist/#mirrorlist/g' -e 's/^#baseurl=http:\/\/mirror.centos.org\/centos\/$releasever\//baseurl=http:\/\/vault.centos.org\/6.10\//g' /etc/yum.repos.d/CentOS-SCLo-scl-rh.repo && \
    rm /etc/yum.repos.d/CentOS-SCLo-scl.repo && \
    rm /etc/yum.repos.d/CentOS-SCLo-scl-rh.repo

# force update root certificates, in particular might return DST Root CA X3
RUN yum -y update ca-certificates && yum -y reinstall ca-certificates

# Blacklist DST ROOT CA, if any
RUN mkdir -p /etc/pki/ca-trust/source/blacklist/
RUN perl -e 'while(<>){last if $_ =~ m/DST Root CA X3/;}print $_;while(<>){last if length($_)==1;print $_}' </etc/pki/tls/certs/ca-bundle.crt > /etc/pki/ca-trust/source/blacklist/DST_Root_CA_X3.pem

RUN mkdir -p /etc/pki/ca-trust/source/anchors
RUN curl https://letsencrypt.org/certs/isrgrootx1.pem > /etc/pki/ca-trust/source/anchors/ISGR_ROOT_X1.pem
RUN curl https://letsencrypt.org/certs/lets-encrypt-r3.pem > /etc/pki/ca-trust/source/anchors/LETS_ENCRYPT_R3.pem
#RUN perl -e 'while(<>){last if $_ =~ m/ISRG Root X1/;}print $_;while(<>){last if length($_)==1;print $_}' </etc/pki/tls/certs/ca-bundle.crt

# after this command DST ROOT CA should disappear from /etc/pki/tls/certs/ca-bundle.crt
# identity R3 and ISRG Root X1 should start to be present
RUN update-ca-trust && update-ca-trust extract

# validate we are able to manage with https:// cert
RUN curl https://curl.se/ca/cacert.pem

# Force update ruby storage (check location via curl -Lks 'https://git.io/rg-ssl' | ruby )
RUN curl https://curl.se/ca/cacert.pem -o /etc/pki/tls/certs/ca-bundle.crt

# /workaround

RUN pip install awscli boto3 sa-rpm-s3

RUN yum -y install net-snmp-devel python-deltarpm yum-utils libtool rpm-sign createrepo

ADD rpmmacros /root/.rpmmacros

SHELL ["/bin/bash", "-c"]
