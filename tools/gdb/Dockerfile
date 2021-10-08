ARG AGENT_VERSION
FROM datadog/agent:${AGENT_VERSION}

# Install gdb
# Use the debian snapshot repo commented out in the sources.list file, so that we install
# the same versions of the debug packages (that are deps of gdb) as what's originally installed in the image
ARG DEBIAN_REPO_SNAPSHOT_DATE
RUN grep '^\#' /etc/apt/sources.list | sed 's/# //' > /etc/apt/sources.list.tmp \
 && if [ -f ".debian_repo_snapshot_date" ]; then \
    sed -i -r "s/[0-9]+T[0-9]+Z/$(cat .debian_repo_snapshot_date)/" /etc/apt/sources.list.tmp; \
    mv /etc/apt/sources.list.tmp /etc/apt/sources.list; \
elif [ ! -z "$DEBIAN_REPO_SNAPSHOT_DATE"]; then \
    sed -i -r "s/[0-9]+T[0-9]+Z/${DEBIAN_REPO_SNAPSHOT_DATE}/" /etc/apt/sources.list.tmp; \
    mv /etc/apt/sources.list.tmp /etc/apt/sources.list; \
 fi \
 && cat /etc/apt/sources.list \ 
 && apt-get -o Acquire::Check-Valid-Until=false update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" gdb

# Install Go version used to build the Agent, to get appropriate GDB debug script
RUN export GIMME_GO_VERSION=$(agent version | grep --only-matching -E 'go[0-9\.]*' | sed 's/go//') \
 && eval "$(curl -sL https://raw.githubusercontent.com/travis-ci/gimme/master/gimme | bash)"

# Install datadog-agent-dbg at the right version
RUN apt-get install -o Dpkg::Options::="--force-confold" --no-install-recommends -y gnupg dirmngr ca-certificates \
 && apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 A2923DFF56EDA6E76E55E492D3A80E30382E94DE
ARG AGENT_VERSION
ARG APT_REPO=apt.datadoghq.com
ARG APT_DISTRIBUTION=stable
RUN echo "deb https://${APT_REPO}/ ${APT_DISTRIBUTION} 7 6" > /etc/apt/sources.list.d/datadog.list \
 && apt-get update -o Dir::Etc::sourcelist="sources.list.d/datadog.list" \
 && cd /root/ \
 && DEB_AGENT_VERSION=$(echo "${AGENT_VERSION}" | sed "s/-/~/") \
 && apt-get download datadog-agent-dbg=1:${DEB_AGENT_VERSION}-1 \
 && dpkg -i --ignore-depends=datadog-agent ./datadog-agent-dbg*.deb \
 && rm -f ./datadog-agent-dbg*.deb

# Add GDB debug script for libpython
RUN TARGET_FILE=$(find /opt/datadog-agent/.debug/opt/datadog-agent/embedded/lib/ -name 'libpython3.*0.dbg' | sed 's/.dbg/-gdb.py/') \
 && curl https://raw.githubusercontent.com/python/cpython/master/Tools/gdb/libpython.py -o ${TARGET_FILE}

# Configure  gdb
COPY .gdbinit /root/

ENTRYPOINT ["gdb", "/opt/datadog-agent/bin/agent/agent"]
