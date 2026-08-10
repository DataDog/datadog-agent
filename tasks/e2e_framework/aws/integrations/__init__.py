from invoke.collection import Collection

from tasks.e2e_framework.aws.integrations.aws_neuron import collection as aws_neuron_collection
from tasks.e2e_framework.aws.integrations.dell_powerflex import collection as dell_powerflex_collection
from tasks.e2e_framework.aws.integrations.etcd import collection as etcd_collection
from tasks.e2e_framework.aws.integrations.kafka import collection as kafka_collection
from tasks.e2e_framework.aws.integrations.lustre import collection as lustre_collection
from tasks.e2e_framework.aws.integrations.postgres import collection as postgres_collection
from tasks.e2e_framework.aws.integrations.redisdb import collection as redisdb_collection

collection = Collection("integrations")
collection.add_collection(redisdb_collection)
collection.add_collection(postgres_collection)
collection.add_collection(kafka_collection)
collection.add_collection(etcd_collection)
collection.add_collection(aws_neuron_collection)
collection.add_collection(lustre_collection)
collection.add_collection(dell_powerflex_collection)
