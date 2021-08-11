# This script is intended to be ran on a lambda script to stop all molecule ec2 instances that has been up for more than 150 min

import json
import boto3
import math
from datetime import datetime

terminate_after_x_minutes = 150

ec2 = boto3.resource('ec2')
ec2client = boto3.client('ec2')

def lambda_handler(event, context):
    response = ec2client.describe_instances(
        Filters=[
            {
                'Name': 'tag:usage',
                'Values': [
                    'molecule-build'
                ]
            },
            {
                'Name': 'tag:dev',
                'Values': [
                    'false'
                ]
            },
            {
                'Name': 'instance-state-name',
                'Values': [
                    'running'
                ]
            },
        ]
   )

    delete_instances = []

    for reservation in response['Reservations']:
        for instance in reservation['Instances']:
            current_time_seconds = datetime.today().timestamp()
            instance_time_seconds = instance['LaunchTime'].timestamp()
            difference = math.ceil((current_time_seconds - instance_time_seconds + 1) / 60)
            print("Instance " + instance['InstanceId'] + " has been running for " + str(difference) + " minutes")
            if difference >= terminate_after_x_minutes:
                print("Adding " + instance['InstanceId'] + " to the terminating list")
                delete_instances.append(instance['InstanceId'])

    if len(delete_instances) > 0:
        ec2.instances.filter(
            Filters=[
                {
                    'Name': 'tag:usage',
                    'Values': [
                        'molecule-build'
                    ]
                },
                {
                    'Name': 'tag:dev',
                    'Values': [
                        'false'
                    ]
                },
                {
                    'Name': 'instance-state-name',
                    'Values': [
                        'running'
                    ]
                },
                {
                    'Name': 'instance-id',
                    'Values': delete_instances
                },
            ]
        ).stop()

    return {
        "statusCode": 200,
        "terminated": delete_instances
    }
