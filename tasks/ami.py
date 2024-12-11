import os

from invoke import task


@task
def launch_instance(_ctx, ami_id, key_name, instance_type='t2.medium'):
    """
    Launch an instance from an AMI.

    Example:
    Run: aws-vault exec sso-agent-qa-account-admin -- deva ami.launch-instance --ami-id ami-0eef9d92ec044bc94 --key-name your-key-name
    Then: ssh -i ~/.ssh/your-key.pem user@ip
    """
    import boto3

    ec2 = boto3.client('ec2')
    response = ec2.run_instances(
        ImageId=ami_id,
        InstanceType=instance_type,
        KeyName=key_name,
        MaxCount=1,
        MinCount=1,
        NetworkInterfaces=[
            {
                "SubnetId": "subnet-0f1ca3e929eb3fb8b",
                "DeviceIndex": 0,
                "AssociatePublicIpAddress": False,
                "Groups": ["sg-070023ab71cadf760"],
            },
        ],
        TagSpecifications=[
            {
                'ResourceType': 'instance',
                'Tags': [
                    {'Key': 'Name', 'Value': f"dd-agent-{os.environ.get('USER', os.environ.get('USERNAME'))}-{ami_id}"},
                ],
            },
        ],
    )

    instance_id = response['Instances'][0]['InstanceId']
    print(f"Instance {instance_id} launched from AMI {ami_id}")
    print(f"IP address: {response['Instances'][0]['PrivateIpAddress']}")


@task
def create_ami(_ctx, instance_id, ami_name, origin_ami, os, usage="test-ami"):
    """
    Create an AMI from a running instance.

    Example: aws-vault exec sso-agent-qa-account-admin -- deva ami.create-ami --instance-id i-054d463dee21bd56f --ami-name test-ami --origin-ami ami-0eef9d92ec044bc94 --os debian-12-x86_64
    """
    import boto3

    ec2 = boto3.client('ec2')
    response = ec2.create_image(
        InstanceId=instance_id,
        Name=ami_name,
        TagSpecifications=[
            {
                'ResourceType': 'image',
                'Tags': [
                    {
                        'Key': 'Usage',
                        'Value': usage,
                    },
                    {
                        'Key': 'OriginAmi',
                        'Value': origin_ami,
                    },
                    {
                        'Key': 'OS',
                        'Value': os,  # <os-version>-<architecture>, ie: debian-12-x86_64
                    },
                ],
            },
        ],
    )

    ami_id = response['ImageId']
    print(f"AMI {ami_id} created from instance {instance_id}")


@task
def delete_ami(_ctx, ami_id):
    """
    Delete an AMI.

    Example: aws-vault exec sso-agent-qa-account-admin -- deva ami.delete-ami --ami-id ami-0890dd73c014b3a84
    """
    import boto3

    ec2 = boto3.client('ec2')
    ec2.deregister_image(
        ImageId=ami_id,
    )
    print(f"AMI {ami_id} deleted")
