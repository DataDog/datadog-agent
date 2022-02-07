FROM arm64v8/ubuntu:focal

ARG DOCKER_HOST_USER
ARG DOCKER_HOST_USER_HOME

RUN apt-get update
RUN DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC apt-get -y install tzdata
RUN apt-get install -y curl git sudo \
      build-essential cmake gdb lcov pkg-config \
      libbz2-dev libffi-dev libgdbm-dev libgdbm-compat-dev liblzma-dev \
      libncurses5-dev libreadline6-dev libsqlite3-dev libssl-dev \
      lzma lzma-dev tk-dev uuid-dev zlib1g-dev libsystemd-dev libpq-dev

WORKDIR /tmp

ENV GO_VERSION_FILE="go1.17.6.linux-arm64.tar.gz"
RUN curl -L -O https://go.dev/dl/${GO_VERSION_FILE}
RUN tar -C /usr/local -xzf ${GO_VERSION_FILE}
ENV PATH="${PATH}:/usr/local/go/bin"

# devtools
RUN apt-get install -y docker.io telnet vim

# sqlserver dependencies
RUN apt-get install -y unixodbc unixodbc-dev freetds-dev freetds-bin tdsodbc

# python3.8 build from source because ubuntu built-in python has issues like not having the development
# headers. If you install python-dev then it's not quite the same version which causes problems
# when building the agent python extensions.
RUN curl -L -O https://www.python.org/ftp/python/3.8.12/Python-3.8.12.tgz
RUN tar -xzf Python-3.8.12.tgz
WORKDIR /tmp/Python-3.8.12
RUN ./configure --enable-shared --enable-optimizations --with-lto --prefix=/opt/Python-3.8.12
RUN make && make install

# only root can call the docker socket to replace the default docker and docker-compose executables
# with bash wrappers that run them as root
RUN /opt/Python-3.8.12/bin/python3 -m pip install docker-compose
RUN mv /usr/bin/docker /usr/bin/docker-orig
RUN mv /opt/Python-3.8.12/bin/docker-compose /opt/Python-3.8.12/bin/docker-compose-orig
RUN echo "#!/usr/bin/env bash" >> /usr/bin/docker
RUN echo "sudo -E docker-orig \"\$@\"" >> /usr/bin/docker
RUN echo "#!/usr/bin/env bash" >> /usr/bin/docker-compose
RUN echo "sudo -E /opt/Python-3.8.12/bin/docker-compose-orig \"\$@\"" >> /usr/bin/docker-compose
RUN chmod a+x /usr/bin/docker /usr/bin/docker-compose

# in order to make the integrations-core tooling work better
RUN useradd -rm -d ${DOCKER_HOST_USER_HOME} -s /bin/bash -g root -G sudo,docker -u 503 ${DOCKER_HOST_USER}
RUN echo '%sudo ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers
USER ${DOCKER_HOST_USER}

ENV PATH "${DOCKER_HOST_USER_HOME}/.local/bin:/opt/Python-3.8.12/bin:$PATH"
RUN python3 -m pip install -U pip

WORKDIR ${DOCKER_HOST_USER_HOME}/dd
RUN git clone https://github.com/DataDog/datadog-agent.git
RUN git clone https://github.com/DataDog/integrations-core.git
RUN python3 -m pip install -r ${DOCKER_HOST_USER_HOME}/dd/datadog-agent/requirements.txt
# exclude dependencies which are difficult to install
RUN cat ${DOCKER_HOST_USER_HOME}/dd/integrations-core/datadog_checks_base/datadog_checks/base/data/agent_requirements.in | grep -v pymqi | grep -v aerospike | grep -v kerberos > /tmp/agent_requirements.in
RUN python3 -m pip install -r /tmp/agent_requirements.in
RUN python3 -m pip install -e "${DOCKER_HOST_USER_HOME}/dd/integrations-core/datadog_checks_dev[cli]"
RUN python3 -m pip install -e "${DOCKER_HOST_USER_HOME}/dd/integrations-core/sqlserver"
RUN python3 -m pip install -e "${DOCKER_HOST_USER_HOME}/dd/integrations-core/postgres"
RUN python3 -m pip install -e "${DOCKER_HOST_USER_HOME}/dd/integrations-core/mysql"
RUN cd ${DOCKER_HOST_USER_HOME}/dd/datadog-agent && invoke install-tools
RUN cd ${DOCKER_HOST_USER_HOME}/dd/datadog-agent && invoke deps
# for some reason it can't find libdatadog-agent-three.so even though
# ${DOCKER_HOST_USER_HOME}/dd/datadog-agent/dev/lib  is in the RUNPATH
# (see objdump -p ${DOCKER_HOST_USER_HOME}/dd/datadog-agent/bin/agent/agent)
# so we need to set this
ENV LD_LIBRARY_PATH "${DOCKER_HOST_USER_HOME}/dd/datadog-agent/dev/lib:$LD_LIBRARY_PATH"
