#!/bin/sh

usage()
{
    echo "Usage: $0 [-i <string>] [-n <0|1>]" 1>&2
    echo "       -i image: docker image to scan for vulnerabilities"
    echo "       -n notify-medium: Whether or not a slack message is sent for medium vulnerabilities"
    exit 1
}

IMAGE=""
NOTIFY=0

while getopts ":i:n:" o; do
    case "${o}" in
        i)
            IMAGE=${OPTARG}
            ;;
        n)
            NOTIFY=${OPTARG}
            ;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND-1))

if [ -z "${IMAGE}" ] || [ -z "${NOTIFY}" ]; then
    usage
fi
if [ ! $NOTIFY = 0 ] && [ ! $NOTIFY = 1 ]; then
    usage
fi

START="false"
FILE="anchore-scan.txt"
ANCHORE="anchore/engine-cli:v0.9.2"
ANCHORE_DOCKER_INVOKE="docker run --rm -a stdout -e ANCHORE_CLI_USER=${ANCHORE_CLI_USER} -e ANCHORE_CLI_PASS=${ANCHORE_CLI_PASS} -e ANCHORE_CLI_URL=${ANCHORE_CLI_URL} ${ANCHORE}"
CURL="curlimages/curl:7.79.1"
CURL_DOCKER_INVOKE="docker run --rm -a stdout ${CURL}"

${ANCHORE_DOCKER_INVOKE} anchore-cli image add "$IMAGE" > /dev/null
${ANCHORE_DOCKER_INVOKE} anchore-cli image wait "$IMAGE" > /dev/null
${ANCHORE_DOCKER_INVOKE} anchore-cli image vuln --vendor-only false "$IMAGE" all > $FILE
${ANCHORE_DOCKER_INVOKE} anchore-cli evaluate check "$IMAGE" --policy "cluster-agent-04x" --detail
# --policy "stackstate-default"

if [ ! -f ${FILE} ]; then
    echo "File ${FILE} not found!"
    exit 1
fi

MESSAGE=""
while IFS= read -r line || [ ! -z "$line" ]; do

    if [ "${START}" = "true" ]; then
        # Here we parse the vulns
        VULN_ID=$(echo $line | awk '{print $1}')
        VULN_LVL=$(echo $line | awk '{print $3}')
        if [ "${VULN_LVL}" = "Critical" ] ||  [ "${VULN_LVL}" = "High" ]; then
            MESSAGE="${MESSAGE}${VULN_LVL} Vulnerability (${VULN_ID}) detected in docker image ${IMAGE}.\n"
        fi

        if [ "${VULN_LVL}" = "Medium" ] && [ $NOTIFY = 1 ]; then
            MESSAGE="${MESSAGE}${VULN_LVL} Vulnerability (${VULN_ID}) detected in docker image ${IMAGE}.\n"
        fi

    elif [ ! -z "$(echo $line | grep "Vulnerability")" ] && [ "${START}" = "false" ]; then
        START="true"
    fi
done < $FILE

if [ ! -z "${MESSAGE}" ]; then
    ${CURL_DOCKER_INVOKE} -X POST -H 'Content-type: application/json' --data '{"text":"'"${MESSAGE}"'"}' ${ANCHORE_WEBHOOK}
    # echo ${MESSAGE}
fi

rm $FILE
