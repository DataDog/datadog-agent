#!/bin/bash

log() {
    echo "$(date +"%D %T") [INFO] $1" >> /usr/local/bin/verify-or-create-topics.log
}

log "Running verify or create topics script"

if [[ -z "$KAFKA_CREATE_TOPICS" ]]; then
    log "KAFKA_CREATE_TOPICS env variable not found"
    exit 0
fi

if [[ -z "$START_TIMEOUT" ]]; then
    START_TIMEOUT=600
fi

start_timeout_exceeded=false
count=0
step=10
while true; do
    kafka-topics.sh --bootstrap-server localhost:$KAFKA_PORT --version
#    netstat -lnt | grep -q $KAFKA_PORT
    if [ $? -eq 0 ]; then
        break
    fi
    log "Waiting for Kafka to be ready"
    sleep $step;
    count=$((count + step))
    if [ $count -gt $START_TIMEOUT ]; then
        start_timeout_exceeded=true
        break
    fi
done

if $start_timeout_exceeded; then
    log "Not able to auto-create topic (waited for $START_TIMEOUT sec)"
    exit 1
fi

log "Kafka is now ready"

# Retrieve and split the defined $KAFKA_CREATE_TOPICS string
IFS="${KAFKA_CREATE_TOPICS_SEPARATOR-,}" read -ra DEFINED_TOPICS <<< "$KAFKA_CREATE_TOPICS"

# Retrieve the existing kafka topics
ACTIVE_TOPICS="$(/opt/kafka/bin/kafka-topics.sh --list --zookeeper zookeeper | grep -v __consumer_offsets | wc -l)"

log "Active Topic Count: ${ACTIVE_TOPICS}"
log "Defined Topic Count: ${#DEFINED_TOPICS[@]}"

if [[ ${ACTIVE_TOPICS} -ge ${#DEFINED_TOPICS[@]} ]]
then
    # Healthy
    log "Healthy"
    log "Exit Code 0"

    exit 0
else
    # UnHealthy
    log "UnHealthy"

    # Expected format:
    #   name:partitions:replicas:cleanup.policy

    IFS="${KAFKA_CREATE_TOPICS_SEPARATOR-,}"; for topicToCreate in $KAFKA_CREATE_TOPICS; do
        log "Creating topics: $topicToCreate ..."
        IFS=':' read -r -a topicConfig <<< "$topicToCreate"
        config=
        if [ -n "${topicConfig[3]}" ]; then
            config="--config=cleanup.policy=${topicConfig[3]}"
        fi

        COMMAND="JMX_PORT='' ${KAFKA_HOME}/bin/kafka-topics.sh \\
            --create \\
            --zookeeper ${KAFKA_ZOOKEEPER_CONNECT} \\
            --topic ${topicConfig[0]} \\
            --partitions ${topicConfig[1]} \\
            --replication-factor ${topicConfig[2]} \\
            ${config} \\
            --if-not-exists"
        eval "${COMMAND}"
    done

    log "Exit Code 1"
    # Force unhealthy exit to allow the health check to rerun
    exit 1
fi

