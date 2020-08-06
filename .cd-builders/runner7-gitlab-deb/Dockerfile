FROM docker.io/stackstate/stackstate-agent-runner-circle:deb7_latest

RUN apt-get install -y nano libtool-bin libelf-dev
RUN echo 'export PATH=$HOME/bin:$PATH' >> /root/.bashrc

RUN echo 'source /usr/local/rvm/scripts/rvm' >> /root/.bashrc
RUN echo 'source /usr/local/rvm/scripts/rvm' >> /root/.bash_profile

#RUN pip install awscli boto3 sa-rpm-s3
#RUN apt-get install -y libkrb5-dev unixodbc-dev rpm python-deltarpm createrepo yum-utils

#ADD rpmmacros /root/.rpmmacros
SHELL ["/bin/bash", "-c"]
