ARG AGENT_VERSION
FROM datadog/agent:${AGENT_VERSION}

RUN apt-get update
RUN rm -vf /etc/ssl/openssl.cnf
RUN DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC apt-get install -y gdb build-essential strace less vim

# Install go
RUN curl -fSL -o golang.tgz https://go.dev/dl/go1.20.8.linux-amd64.tar.gz
RUN tar xzvf golang.tgz
RUN ln -s /go /goroot

# Install go-delve
RUN /go/bin/go install github.com/go-delve/delve/cmd/dlv@latest

# Install datadog-agent-dbg at the right version
RUN apt-get install -o Dpkg::Options::="--force-confold" --no-install-recommends -y gnupg dirmngr ca-certificates \
 && apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 33EE313BAD9589B7
ARG AGENT_VERSION
ARG APT_REPO=apt.datadoghq.com
ARG APT_DISTRIBUTION=stable
RUN echo "deb https://${APT_REPO}/ ${APT_DISTRIBUTION} 7 6" > /etc/apt/sources.list.d/datadog.list \
 && apt-get update -o Dir::Etc::sourcelist="sources.list.d/datadog.list" \
 && cd /root/ \
 && DEB_AGENT_VERSION=$(echo "${AGENT_VERSION}" | sed "s/-/~/") \
 && apt-get download datadog-agent-dbg=1:${DEB_AGENT_VERSION}-1 \
 && dpkg -X ./datadog-agent-dbg*.deb / \
 && rm -f ./datadog-agent-dbg*.deb

# Add GDB debug script for libpython
RUN TARGET_FILE=$(find /opt/datadog-agent/.debug/opt/datadog-agent/embedded/lib/ -name 'libpython3.*0.dbg' | sed 's/.dbg/-gdb.py/') \
 && curl https://raw.githubusercontent.com/python/cpython/master/Tools/gdb/libpython.py -o ${TARGET_FILE}

RUN mkdir -p /usr/lib/debug/.build-id/$(readelf -n /opt/datadog-agent/bin/agent/agent | perl -nE 'say $1 if /Build ID: (..)(.*)/')

RUN cp /opt/datadog-agent/.debug/opt/datadog-agent/bin/agent/agent.dbg \
       /usr/lib/debug/.build-id/$(readelf -n /opt/datadog-agent/bin/agent/agent | perl -nE 'say "$1/$2.debug" if /Build ID: (..)(.*)/')

# Configure  gdb
COPY .gdbinit /root/

ENTRYPOINT ["gdb", "/opt/datadog-agent/bin/agent/agent"]
