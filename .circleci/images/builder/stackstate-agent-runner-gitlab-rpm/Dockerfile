FROM stackstate/stackstate-agent-runner-circle:rpm_base.latest

RUN pip install awscli boto3

RUN yum -y install net-snmp-devel python-deltarpm yum-utils libtool rpm-sign createrepo

# todo: if works, work rpm-s3
RUN cd /usr/local && \
    git clone https://github.com/Voronenko/rpm-s3.git --recursive
ENV PATH /usr/local/rpm-s3/bin:$PATH
ADD rpmmacros /root/.rpmmacros
