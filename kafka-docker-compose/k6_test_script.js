/*
This is a k6 test script that imports the xk6-kafka and
tests Kafka with a 200 byte array messages per iteration.
*/

import { check } from "k6";
import { Writer, Reader, Connection, SchemaRegistry, SCHEMA_TYPE_BYTES } from "k6/x/kafka"; // import kafka extension

const brokers = ["localhost:9092"];
const topic = "xk6_kafka_byte_array_topic";

const writer = new Writer({
    brokers: brokers,
    topic: topic,
    autoCreateTopic: true,
});
const reader = new Reader({
    brokers: brokers,
    topic: topic,
});
const connection = new Connection({
    address: brokers[0],
});
const schemaRegistry = new SchemaRegistry();

if (__VU == 0) {
    connection.createTopic({ topic: topic });
}

const payload = "byte array payload";

export default function () {
    for (let index = 0; index < 100; index++) {
        let messages = [
            {
                // The data type of the key is a string
                key: schemaRegistry.serialize({
                    data: Array.from("test-id-abc-" + index, (x) => x.charCodeAt(0)),
                    schemaType: SCHEMA_TYPE_BYTES,
                }),
                // The data type of the value is a byte array
                value: schemaRegistry.serialize({
                    data: Array.from(payload, (x) => x.charCodeAt(0)),
                    schemaType: SCHEMA_TYPE_BYTES,
                }),
            },
            {
                key: schemaRegistry.serialize({
                    data: Array.from("test-id-def-" + index, (x) => x.charCodeAt(0)),
                    schemaType: SCHEMA_TYPE_BYTES,
                }),
                value: schemaRegistry.serialize({
                    data: Array.from(payload, (x) => x.charCodeAt(0)),
                    schemaType: SCHEMA_TYPE_BYTES,
                }),
            },
        ];

        writer.produce({
            messages: messages,
        });
    }

    // Read 10 messages only
    let messages = reader.consume({ limit: 10 });
    check(messages, {
        "10 messages returned": (msgs) => msgs.length == 10,
        "key starts with 'test-id-' string": (msgs) =>
            String.fromCharCode(
                ...schemaRegistry.deserialize({ data: msgs[0].key, schemaType: SCHEMA_TYPE_BYTES })
            ).startsWith("test-id-"),
        "value is correct": (msgs) =>
            String.fromCharCode(
                ...schemaRegistry.deserialize({
                    data: msgs[0].value,
                    schemaType: SCHEMA_TYPE_BYTES,
                })
            ) == payload,
    });
}

export function teardown(data) {
    if (__VU == 0) {
        // Delete the topic
        connection.deleteTopic(topic);
    }
    writer.close();
    reader.close();
    connection.close();
}
