#!/bin/bash

# Usage:
#
#   ssh-keygen -f ~/.ssh/sidescanner -t ed25519 -C "sidescannerdebug@datadoghq.com"
#   STACK_OWNER=$(whoami) aws-vault exec sso-sandbox-account-admin -- ./agentless-scanner-deploy.sh
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
STACK_NAME="DatadogSideScanner-${STACK_OWNER}"
STACK_AWS_REGION="us-east-1"
STACK_PUBLIC_KEY=$(cat "$HOME/.ssh/sidescanner.pub")
STACK_INSTALL_SH=$(base64 -w 0 < "${BASEDIR}/userdata.sh")
STACK_USER_DATA=$(cat <<EOF | base64 -w 0
#!/bin/bash
echo "$STACK_INSTALL_SH" | base64 -d > userdata.sh
chmod +x userdata.sh
DD_API_KEY="$STACK_API_KEY" ./userdata.sh "${STACK_OWNER}"
EOF
)
STACK_TEMPLATE=$(cat <<EOF
AWSTemplateFormatVersion: '2010-09-09'
Description: Create an EC2 instance in an isolated VPC for Datadog SideScanner

Parameters:
  DatadogSideScannerAMIId:
    Type: 'AWS::SSM::Parameter::Value<AWS::EC2::Image::Id>'
    Default: '/aws/service/canonical/ubuntu/server/22.04/stable/current/arm64/hvm/ebs-gp2/ami-id'

  DatadogSideScannerInstanceType:
    Type: String
    Default: '${STACK_INSTANCE_TYPE}'

Resources:
  DatadogSideScannerKeyPair:
    Type: AWS::EC2::KeyPair
    Properties:
      KeyName: ${STACK_NAME}SSHKeyPair
      PublicKeyMaterial: "${STACK_PUBLIC_KEY}"

  DatadogSideScannerAccessRole:
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

  DatadogSideScannerAccessPolicy:
    Type: AWS::IAM::Policy
    Properties:
      PolicyName: ${STACK_NAME}AccessPolicy
      Roles:
        - !Ref DatadogSideScannerAccessRole
      PolicyDocument:
        Version: '2012-10-17'
        Statement:
        - Action: ec2:CreateTags
          Condition:
            StringEquals:
              ec2:CreateAction:
              - CreateSnapshot
              - CreateVolume
          Effect: Allow
          Resource:
          - arn:aws:ec2:*:*:volume/*
          - arn:aws:ec2:*:*:snapshot/*
          Sid: DatadogSideScannerResourceTagging
        - Action: ec2:CreateSnapshot
          Condition:
            StringNotEquals:
              aws:ResourceTag/DatadogSideScanner: 'false'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:volume/*
          Sid: DatadogSideScannerVolumeSnapshotCreation
        - Action: ec2:CreateSnapshot
          Condition:
            ForAllValues:StringLike:
              aws:TagKeys: DatadogSideScanner*
            StringEquals:
              aws:RequestTag/DatadogSideScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:snapshot/*
          Sid: DatadogSideScannerSnapshotCreation
        - Action:
          - ec2:ModifySnapshotAttribute
          - ec2:DescribeSnapshotAttribute
          - ec2:DeleteSnapshot
          - ebs:ListSnapshotBlocks
          - ebs:ListChangedBlocks
          - ebs:GetSnapshotBlock
          Condition:
            StringEquals:
              aws:ResourceTag/DatadogSideScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:snapshot/*
          Sid: DatadogSideScannerSnapshotAccessAndCleanup
        - Action: ec2:DescribeSnapshots
          Effect: Allow
          Resource: "*"
          Sid: DatadogSideScannerDescribeSnapshots
        - Action: ec2:CreateVolume
          Condition:
            ForAllValues:StringLike:
              aws:TagKeys: DatadogSideScanner*
            StringEquals:
              aws:RequestTag/DatadogSideScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:volume/*
          Sid: DatadogSideScannerVolumeCreation
        - Action:
          - ec2:DetachVolume
          - ec2:AttachVolume
          Condition:
            StringEquals:
              aws:ResourceTag/DatadogSideScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:instance/*
          Sid: DatadogSideScannerVolumeAttachToInstance
        - Action:
          - ec2:DetachVolume
          - ec2:DeleteVolume
          - ec2:AttachVolume
          Condition:
            StringEquals:
              aws:ResourceTag/DatadogSideScanner: 'true'
          Effect: Allow
          Resource: arn:aws:ec2:*:*:volume/*
          Sid: DatadogSideScannerVolumeAttachAndDelete
        - Action: lambda:GetFunction
          Effect: Allow
          Resource: arn:aws:lambda:*:*:function:*
          Sid: GetLambdaDetails
        - Action: ec2:DescribeInstances
          Effect: Allow
          Resource: '*'
          Sid: DatadogSideScannerOfflineMode

  DatalogSideScannerInstanceProfile:
    Type: AWS::IAM::InstanceProfile
    Properties:
      Path: /
      Roles:
        - !Ref DatadogSideScannerAccessRole

  DatadogSideScannerInstance:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: !Ref DatadogSideScannerInstanceType
      KeyName: !Ref DatadogSideScannerKeyPair
      IamInstanceProfile: !Ref DatalogSideScannerInstanceProfile
      UserData: ${STACK_USER_DATA}
      SecurityGroupIds:
        - ${STACK_SECURITY_GROUP}
      SubnetId: ${STACK_SUBNET_ID}
      ImageId: !Ref DatadogSideScannerAMIId
      Tags:
        - Key: Name
          Value: ${STACK_NAME}RootInstance
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
    --logical-resource-id "DatadogSideScannerInstance" \
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
