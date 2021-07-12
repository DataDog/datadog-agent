FROM docker.io/stackstate/stackstate-agent-runner-circle:deb7_latest

RUN DEBIAN_FRONTEND=noninteractive apt-get install -y nano libtool-bin libelf-dev gnupg2 locales
RUN echo 'export PATH=$HOME/bin:$PATH' >> /root/.bashrc

RUN echo 'source /usr/local/rvm/scripts/rvm' >> /root/.bashrc
RUN echo 'source /usr/local/rvm/scripts/rvm' >> /root/.bash_profile
RUN echo 'gem: --no-document' >> /root/.gemrc

RUN bash -c "source /usr/local/rvm/scripts/rvm && gem install deb-s3"

#RUN pip install awscli boto3 sa-rpm-s3
ADD rpmmacros /root/.rpmmacros
# yum-utils #https://pkgs.org/download/yum-utils
#RUN apt-get install -y libkrb5-dev unixodbc-dev rpm python-deltarpm createrepo bison yum-utils

RUN sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && \
    locale-gen
ENV LANG en_US.UTF-8  
ENV LANGUAGE en_US:en  
ENV LC_ALL en_US.UTF-8 

SHELL ["/bin/bash", "-c"]
