#!/usr/bin/env bash
set -euo pipefail

case "$(basename "$0")" in
    agent)
        tags=(
            bundle_agent
            bundle_installer
            consul
            containerd
            no_dynamic_plugins
            cri
            crio
            datadog.no_waf
            docker
            ec2
            etcd
            fargateprocess
            grpcnotrace
            jetson
            jmx
            kubeapiserver
            kubelet
            netcgo
            nvml
            oracle
            orchestrator
            otlp
            podman
            python
            sds
            systemd
            trivy
            trivy_no_javadb
            zk
            zlib
            zstd
        )
        ;;
    process-agent)
        tags=(
            containerd
            no_dynamic_plugins
            cri
            crio
            datadog.no_waf
            ec2
            docker
            fargateprocess
            grpcnotrace
            kubelet
            netcgo
            podman
            zlib
            zstd
        )
        ;;
    trace-agent)
        tags=(
            docker
            containerd
            grpcnotrace
            no_dynamic_plugins
            datadog.no_waf
            kubeapiserver
            kubelet
            otlp
            netcgo
            podman
        )
        ;;
    *)
        printf "%s unknown agent name\n" "$0" >&2
        exit 1
        ;;
esac

function remove_tag {
    local tag="$1"
    for i in "${!tags[@]}"; do
        if [[ "${tags[i]}" == "$tag" ]]; then
            unset 'tags[i]'
            break
        fi
    done
}

# We’ve disabled those build tags to speed up the build process during the PoC.
remove_tag "consul"
remove_tag "crio"
remove_tag "etcd"
remove_tag "fargateprocess"
remove_tag "oracle"
remove_tag "orchestrator"
remove_tag "podman"
remove_tag "trivy"
remove_tag "trivy_no_javadb"

printf "Dynamic build tags:\n"

function found {
    local tag="$1"
    printf "\t%s:\t%senabled%s\n" "$tag" "$(tput setaf 2)" "$(tput sgr0)"
}

function not_found {
    local tag="$1"
    printf "\t%s:\t%sdisabled%s\n" "$tag" "$(tput setaf 1)" "$(tput sgr0)"
    remove_tag "$tag"
}

# Detect containerd
if [[ -S /run/containerd/containerd.sock ]]; then
    found "containerd"
else
    not_found "containerd"
fi

# Detect CRI
if [[ -S /var/run/cri.sock ]]; then
    found "cri"
else
    not_found "cri"
fi

# Detect docker
if [[ -n "${DOCKER_HOST+x}" ]] || [[ -S /var/run/docker.sock ]]; then
    found "docker"
else
    not_found "docker"
fi

# Detect EC2
#http_code="$(curl -s -o /dev/null -w '%{http_code}' -X PUT 'http://169.254.169.254/latest/api/token' -H 'X-aws-ec2-metadata-token-ttl-seconds: 21600')"
http_code="$(curl -s -o /dev/null -w '%{http_code}' -H "X-aws-ec2-metadata-token: $(curl -s -X PUT 'http://169.254.169.254/latest/api/token' -H 'X-aws-ec2-metadata-token-ttl-seconds: 21600')" http://169.254.169.254/latest/meta-data/)"
if [[ "$http_code" == "200" ]]; then
    found "ec2"
else
    not_found "ec2"
fi

# Detect kubeapiserver
if ! [[ -f /var/run/secrets/kubernetes.io/serviceaccount/ca.crt ]] || ! [[ -f /var/run/secrets/kubernetes.io/serviceaccount/token ]]; then
    not_found "kubeapiserver"
else
    http_code="$(curl -s -o /dev/null -w '%{http_code}' --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt -k -H "Authorization: Bearer $(</var/run/secrets/kubernetes.io/serviceaccount/token)" https://kubernetes.default.svc.cluster.local/apis)"
    if [[ "$http_code" == "200" ]]; then
        found "kubeapiserver"
    else
        not_found "kubeapiserver"
    fi
fi

# Detect kubelet
if ! [[ -f /var/run/secrets/kubernetes.io/serviceaccount/ca.crt ]] || ! [[ -f /var/run/secrets/kubernetes.io/serviceaccount/token ]]; then
    not_found "kubelet"
else
    http_code="$(curl -s -o /dev/null -w '%{http_code}' --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt -k -H "Authorization: Bearer $(</var/run/secrets/kubernetes.io/serviceaccount/token)" https://${DD_KUBERNETES_KUBELET_HOST:-localhost}:10250/pods)"
    if [[ "$http_code" == "200" ]]; then
        found "kubelet"
    else
        not_found "kubelet"
    fi
fi

IFS=,
exec ${EXECUTOR:-/opt/datadog-agent/embedded/bin/executor} --log-level "${EXECUTOR_LOG_LEVEL:-0}" --db "${LINK_DB:-/opt/datadog-agent/embedded/share/datadog-agent/link.db}" --tags "${tags[*]}" --link "${LINK:-/opt/datadog-agent/embedded/bin/link}" -- "$(basename "$0")" $@
