#!/bin/bash

# Usage:
#
#   ssh-keygen -f ~/.ssh/sidescanner -t ed25519 -C "sidescannerdebug@datadoghq.com"
#   STACK_OWNER=$(whoami) \
#   STACK_API_KEY=<api_key> \
#   STACK_SUBNET_ID=<subnet> \
#   STACK_SECURITY_GROUP=<sg> \
#   aws-vault exec sso-sandbox-account-admin -- ./aws-deploy.sh
#
#     export DD_SIDESCANNER_IP="XXX.XXX.XXX.XXX"
#     ssh -i ubuntu@$DD_SIDESCANNER_IP
#

set -e

if [ -z "$STACK_API_KEY" ]; then
  >&2 printf "Please provide a \$STACK_API_KEY\n"
  exit 1
fi

if [ -z "$STACK_OWNER" ]; then
  >&2 printf "Please provide a \$STACK_OWNER\n"
  exit 1
fi

if [ -z "${STACK_SUBNET_ID}" ]; then
  >&2 printf "Please provide a \$STACK_SUBNET_ID\n"
  exit 1
fi

if [ -z "${STACK_SECURITY_GROUP}" ]; then
  >&2 printf "Please provide a \$STACK_SECURITY_GROUP\n"
  exit 1
fi

if [ -z "${STACK_INSTANCE_TYPE}" ]; then
  STACK_INSTANCE_TYPE="t4g.medium"
fi
BASEDIR=$(dirname "$0")

# Stack meta
STACK_NAME="DatadogAgentlessScanner-${STACK_OWNER}"
STACK_AWS_REGION="us-east-1"
STACK_PUBLIC_KEY=$(cat "$HOME/.ssh/sidescanner.pub")
STACK_INSTALL_SH=$(base64 -w 0 < "${BASEDIR}/userdata.sh")
STACK_USER_DATA=$(cat <<EOF | base64 -w 0
#!/bin/bash
echo "$STACK_INSTALL_SH" | base64 -d > userdata.sh
chmod +x userdata.sh
DD_API_KEY="$STACK_API_KEY" DD_API_KEY_DUAL="$STACK_API_KEY_DUAL" ./userdata.sh "${STACK_OWNER}"
EOF
)
STACK_TEMPLATE=$(cat <<EOF
AWSTemplateFormatVersion: '2010-09-09'
Description: Create an EC2 instance in an isolated VPC for Datadog AgentlessScanner

Parameters:
  DatadogAgentlessScannerAMIId:
    Type: 'AWS::SSM::Parameter::Value<AWS::EC2::Image::Id>'
    Default: '/aws/service/canonical/ubuntu/server/22.04/stable/current/arm64/hvm/ebs-gp2/ami-id'

  DatadogAgentlessScannerInstanceType:
    Type: String
    Default: '${STACK_INSTANCE_TYPE}'

Resources:
  DatadogAgentlessScannerKeyPair:
    Type: AWS::EC2::KeyPair
    Properties:
      KeyName: ${STACK_NAME}SSHKeyPair
      PublicKeyMaterial: "${STACK_PUBLIC_KEY}"

  DatadogAgentlessScannerAccessRole:
    Type: AWS::IAM::Role
    Properties:
      AssumeRolePolicyDocument:
        Version: '2012-10-17'
        Statement:
          - Effect: Allow
            Principal:
              Service:
                - ec2.amazonaws.com
            Action:
              - sts:AssumeRole

  DatadogAgentlessScannerAccessPolicy:
    Type: AWS::IAM::Policy
    Properties:
      PolicyName: ${STACK_NAME}AccessPolicy
      Roles:
        - !Ref DatadogAgentlessScannerAccessRole
      PolicyDocument:
        Version: '2012-10-17'
        Statement:
        - Action: ec2:CreateTags
          Condition:
            StringEquals:
              ec2:CreateAction:
              - CreateSnapshot
              - CreateVolume
              - CopySnapshot
          Effect: Allow
          Resource:
          - arn:aws:ec2:*:*:volume/*
          - arn:aws:ec2:*:*:snapshot/*
          Sid: DatadogAgentlessScannerResourceTagging
        - Action: ec2:CreateSnapshot
          Condition:
            StringNotEquals:
              aws:ResourceTag/DatadogAgentlessScanner: 'false'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:volume/*
          Sid: DatadogAgentlessScannerVolumeSnapshotCreation
        - Action: ec2:CreateSnapshot
          Condition:
            ForAllValues:StringLike:
              aws:TagKeys: DatadogAgentlessScanner*
            StringEquals:
              aws:RequestTag/DatadogAgentlessScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:snapshot/*
          Sid: DatadogAgentlessScannerSnapshotCreation
        - Action:
          - ec2:ModifySnapshotAttribute
          - ec2:DescribeSnapshotAttribute
          - ec2:DeleteSnapshot
          - ebs:ListSnapshotBlocks
          - ebs:ListChangedBlocks
          - ebs:GetSnapshotBlock
          Condition:
            StringEquals:
              aws:ResourceTag/DatadogAgentlessScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:snapshot/*
          Sid: DatadogAgentlessScannerSnapshotAccessAndCleanup
        - Action: ec2:DescribeSnapshots
          Effect: Allow
          Resource: "*"
          Sid: DatadogAgentlessScannerDescribeSnapshots
        - Action: ec2:CreateVolume
          Condition:
            ForAllValues:StringLike:
              aws:TagKeys: DatadogAgentlessScanner*
            StringEquals:
              aws:RequestTag/DatadogAgentlessScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:volume/*
          Sid: DatadogAgentlessScannerVolumeCreation
        - Action:
          - ec2:DetachVolume
          - ec2:AttachVolume
          Condition:
            StringEquals:
              aws:ResourceTag/DatadogAgentlessScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:instance/*
          Sid: DatadogAgentlessScannerVolumeAttachToInstance
        - Action:
          - ec2:DetachVolume
          - ec2:DeleteVolume
          - ec2:AttachVolume
          Condition:
            StringEquals:
              aws:ResourceTag/DatadogAgentlessScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:volume/*
          Sid: DatadogAgentlessScannerVolumeAttachAndDelete
        - Action: lambda:GetFunction
          Effect: Allow
          Resource: arn:aws:lambda:*:*:function:*
          Sid: GetLambdaDetails
        - Action: lambda:ListFunctions
          Effect: Allow
          Resource: '*'
          Sid: ListLambdas
        - Action: ec2:DescribeInstances
          Effect: Allow
          Resource: '*'
          Sid: DatadogAgentlessScannerOfflineModeLambdas
        - Action: ec2:DescribeRegions
          Effect: Allow
          Resource: '*'
          Sid: DatadogAgentlessScannerOfflineModeRegions
        - Action: ec2:DescribeVolumes
          Effect: Allow
          Resource: '*'
          Sid: DatadogAgentlessScannerDescribeVolumes
        - Action: ec2:DescribeImages
          Effect: Allow
          Resource: '*'
          Sid: DatadogAgentlessScannerDescribeImages
        - Action: ec2:CopySnapshot
          Effect: Allow
          Condition:
            ForAllValues:StringLike:
              aws:TagKeys: DatadogAgentlessScanner*
            StringEquals:
              aws:RequestTag/DatadogAgentlessScanner: 'true'
          Resource: arn:aws:ec2:*:*:snapshot/*
          Sid: DatadogAgentlessScannerCopySnapshot

  DatadogAgentlessScannerInstanceProfile:
    Type: AWS::IAM::InstanceProfile
    Properties:
      Path: /
      Roles:
        - !Ref DatadogAgentlessScannerAccessRole

  DatadogAgentlessScannerInstance:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: !Ref DatadogAgentlessScannerInstanceType
      KeyName: !Ref DatadogAgentlessScannerKeyPair
      IamInstanceProfile: !Ref DatadogAgentlessScannerInstanceProfile
      BlockDeviceMappings:
      - DeviceName: /dev/sda1
        Ebs:
          DeleteOnTermination: true
          Encrypted: true
          VolumeSize: 30
          VolumeType: gp2
      UserData: ${STACK_USER_DATA}
      SecurityGroupIds:
        - ${STACK_SECURITY_GROUP}
      SubnetId: ${STACK_SUBNET_ID}
      ImageId: !Ref DatadogAgentlessScannerAMIId
      Tags:
        - Key: Name
          Value: ${STACK_NAME}RootInstance
        - Key: DatadogAgentlessScanner
          Value: "true"
        - Key: please_keep_my_resource
          Value: "true"
EOF
)

printf "deleting stack %s..." "$STACK_NAME"
aws cloudformation delete-stack --stack-name "$STACK_NAME" --region "$STACK_AWS_REGION"
aws cloudformation wait stack-delete-complete --stack-name "$STACK_NAME" --region "$STACK_AWS_REGION"
printf "\n"

printf "creating stack %s..." "${STACK_NAME}"
STACK_ARN=$(aws cloudformation create-stack \
  --stack-name "$STACK_NAME" \
  --region "$STACK_AWS_REGION" \
  --template-body "$STACK_TEMPLATE" \
  --capabilities CAPABILITY_NAMED_IAM \
  --query 'StackId' \
  --output text)


printf " waiting for stack \"%s\" to complete...\n" "${STACK_ARN}"
printf "> https://%s.console.aws.amazon.com/cloudformation/home?region=%s#/stacks/stackinfo?stackId=%s\n" "${STACK_AWS_REGION}" "${STACK_AWS_REGION}" "${STACK_ARN}"

if aws cloudformation wait stack-create-complete --stack-name "$STACK_NAME" --region "$STACK_AWS_REGION" > /dev/null;
then
  printf "\n"
  STACK_INSTANCE_ID=$(aws cloudformation describe-stack-resource \
    --stack-name "$STACK_NAME" \
    --logical-resource-id "DatadogAgentlessScannerInstance" \
    --query 'StackResourceDetail.PhysicalResourceId' \
    --output text)

  STACK_INSTANCE_IP=$(aws ec2 describe-instances \
    --instance-ids "$STACK_INSTANCE_ID" \
    --query 'Reservations[0].Instances[0].PrivateIpAddress' \
    --output text)

  echo "stack creation successful | launched instance: ${INSTANCE_ID}."
  echo "export STACK_INSTANCE_IP=\"${STACK_INSTANCE_IP}\""
  echo "ssh -i ~/.ssh/sidescanner ubuntu@\$STACK_INSTANCE_IP"
else
  printf "\n"
  echo "Stack creation failed. Check the AWS CloudFormation console for details."
fi
