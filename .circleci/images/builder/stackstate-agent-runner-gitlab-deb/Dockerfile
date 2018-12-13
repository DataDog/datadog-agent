FROM stackstate/stackstate-agent-runner-circle:latest

RUN pip install awscli boto3
RUN apt-get install -y libkrb5-dev unixodbc-dev rpm python-deltarpm createrepo yum-utils

RUN cd /usr/local && \
    git clone https://github.com/Voronenko/rpm-s3.git --recursive
ENV PATH /usr/local/rpm-s3/bin:$PATH
ADD rpmmacros /root/.rpmmacros
