# To run this docker:
# 1. $> docker build -t ddnolimit
# 2. $> mkdir events # to retrieve generated events
# 3. $> docker run \
#   --cgroupns host \
#   --pid host \
#   --security-opt apparmor:unconfined \
#   --cap-add SYS_ADMIN \
#   --cap-add SYS_RESOURCE \
#   --cap-add SYS_PTRACE \
#   --cap-add NET_ADMIN \
#   --cap-add NET_BROADCAST \
#   --cap-add NET_RAW \
#   --cap-add IPC_LOCK \
#   --cap-add CHOWN \
#   -v /var/run/docker.sock:/var/run/docker.sock:ro \
#   -v /proc/:/host/proc/:ro \
#   -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
#   -v /etc/passwd:/etc/passwd:ro \
#   -v /etc/group:/etc/group:ro \
#   -v /:/host/root:ro \
#   -v /sys/kernel/debug:/sys/kernel/debug \
#   -v /etc/os-release:/etc/os-release \
#   -v $(pwd)/events:/tmp/dd_events
#   -d --name ddnolimit ddnolimit

FROM ubuntu
RUN mkdir -p /opt/datadog-agent/run
COPY ./bin/system-probe/system-probe /system-probe
CMD [ "/system-probe" ]
