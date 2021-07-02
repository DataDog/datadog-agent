#!/bin/bash

if [[ -z "$KAFKA_CREATE_TOPICS" ]]; then
    exit 0
fi

# Retrieve and split the defined $KAFKA_CREATE_TOPICS string
IFS=',' read -ra DEFINED_TOPICS <<< "$KAFKA_CREATE_TOPICS"

# Retrieve the existing kafka topics
ACTIVE_TOPICS="$(/opt/kafka/bin/kafka-topics.sh --list --zookeeper zookeeper | grep -v __consumer_offsets | wc -l)"

# Debug Logs
echo "Active Topic Count: ${ACTIVE_TOPICS}"
echo "Defined Topic Count: ${#DEFINED_TOPICS[@]}"

if [[ ${ACTIVE_TOPICS} -ge ${#DEFINED_TOPICS[@]} ]]
then
    # Healthy
    exit 0
else
    # UnHealthy
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
        echo "Waiting for Kafka to be ready"
        sleep $step;
        count=$((count + step))
        if [ $count -gt $START_TIMEOUT ]; then
            start_timeout_exceeded=true
            break
        fi
    done


    if $start_timeout_exceeded; then
        echo "Not able to auto-create topic (waited for $START_TIMEOUT sec)"
        exit 1
    fi

    echo "Kafka is now ready"

    # Expected format:
    #   name:partitions:replicas:cleanup.policy
    for topicToCreate in $DEFINED_TOPICS; do
        echo "Creating topics: $topicToCreate ..."
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
            ${KAFKA_0_10_OPTS}"
        eval "${COMMAND}"
    done

    # Force unhealthy exit to allow the health check to rerun
    exit 1
fi

