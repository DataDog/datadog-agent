//Worker Node AutoScaling Group
//Now we have everything in place to create and manage EC2 instances that will serve as our worker nodes
//in the Kubernetes cluster. This setup utilizes an EC2 AutoScaling Group (ASG) rather than manually working with
//EC2 instances. This offers flexibility to scale up and down the worker nodes on demand when used in conjunction
//with AutoScaling policies (not implemented here).
//
//First, let us create a data source to fetch the latest Amazon Machine Image (AMI) that Amazon provides with an
//EKS compatible Kubernetes baked in.

data "aws_ami" "eks-worker" {
  filter {
    name   = "name"
    values = ["amazon-eks-node-*"]
  }

  most_recent = true
  owners      = ["602401143452"] # Amazon Account ID
}

# EKS currently documents this required userdata for EKS worker nodes to
# properly configure Kubernetes applications on the EC2 instance.
# We utilize a Terraform local here to simplify Base64 encoding this
# information into the AutoScaling Launch Configuration.
# More information: https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-06-05/amazon-eks-nodegroup.yaml
locals {
  eks-node-userdata = <<USERDATA
#!/bin/bash -xe

CA_CERTIFICATE_DIRECTORY=/etc/kubernetes/pki
CA_CERTIFICATE_FILE_PATH=$CA_CERTIFICATE_DIRECTORY/ca.crt
mkdir -p $CA_CERTIFICATE_DIRECTORY
echo "${aws_eks_cluster.eks-cluster.certificate_authority[0].data}" | base64 -d >  $CA_CERTIFICATE_FILE_PATH
INTERNAL_IP=$(curl -s http://169.254.169.254/latest/meta-data/local-ipv4)
sed -i s,MASTER_ENDPOINT,${aws_eks_cluster.eks-cluster.endpoint},g /var/lib/kubelet/kubeconfig
sed -i s,CLUSTER_NAME,${local.cluster_name},g /var/lib/kubelet/kubeconfig
sed -i s,REGION,${var.AWS_REGION},g /etc/systemd/system/kubelet.service
sed -i s,MAX_PODS,20,g /etc/systemd/system/kubelet.service
sed -i s,MASTER_ENDPOINT,${aws_eks_cluster.eks-cluster.endpoint},g /etc/systemd/system/kubelet.service
sed -i s,INTERNAL_IP,$INTERNAL_IP,g /etc/systemd/system/kubelet.service
DNS_CLUSTER_IP=10.100.0.10
if [[ $INTERNAL_IP == 10.* ]] ; then DNS_CLUSTER_IP=172.20.0.10; fi
sed -i s,DNS_CLUSTER_IP,$DNS_CLUSTER_IP,g /etc/systemd/system/kubelet.service
sed -i s,CERTIFICATE_AUTHORITY_FILE,$CA_CERTIFICATE_FILE_PATH,g /var/lib/kubelet/kubeconfig
sed -i s,CLIENT_CA_FILE,$CA_CERTIFICATE_FILE_PATH,g  /etc/systemd/system/kubelet.service
systemctl daemon-reload
systemctl restart kubelet
USERDATA

}

resource "tls_private_key" "eks_rsa" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "aws_key_pair" "eks-key-pair" {
  key_name   = "eks-deployer-${local.cluster_name}"
  public_key = tls_private_key.eks_rsa.public_key_openssh
}

resource "aws_launch_configuration" "eks-launch-configuration" {
  associate_public_ip_address = true
  iam_instance_profile        = aws_iam_instance_profile.eks-node-instance-profile.name
  image_id                    = data.aws_ami.eks-worker.id
  instance_type               = "t2.small"
  name_prefix                 = "eks-${local.cluster_name}"
  security_groups             = [aws_security_group.eks-nodes-sg.id]
  user_data_base64            = base64encode(local.eks-node-userdata)
  key_name                    = aws_key_pair.eks-key-pair.key_name

  lifecycle {
    create_before_destroy = true
  }
}

//Finally, we create an AutoScaling Group that actually launches EC2 instances based on the
//AutoScaling Launch Configuration.

//NOTE: The usage of the specific kubernetes.io/cluster/* resource tag below is required for EKS
//and Kubernetes to discover and manage compute resources.

resource "aws_autoscaling_group" "eks-autoscaling-group" {
  desired_capacity     = var.SCALING_DESIRED_CAPACITY
  launch_configuration = aws_launch_configuration.eks-launch-configuration.id
  max_size             = 2
  min_size             = 0
  name                 = "eks-${local.cluster_name}"
  vpc_zone_identifier  = [aws_subnet.eks-private.id, aws_subnet.eks-private-2.id]

  tag {
    key                 = "Environment"
    value               = var.CLUSTER_NAME
    propagate_at_launch = true
  }

  tag {
    key                 = "Name"
    value               = "eks-${local.cluster_name}"
    propagate_at_launch = true
  }

  tag {
    key                 = "kubernetes.io/cluster/${local.cluster_name}"
    value               = "owned"
    propagate_at_launch = true
  }
}

//NOTE: At this point, your Kubernetes cluster will have running masters and worker nodes, however, the worker nodes will
//not be able to join the Kubernetes cluster quite yet. The next section has the required Kubernetes configuration to
//enable the worker nodes to join the cluster.

//Required Kubernetes Configuration to Join Worker Nodes
//The EKS service does not provide a cluster-level API parameter or resource to automatically configure the underlying
//Kubernetes cluster to allow worker nodes to join the cluster via AWS IAM role authentication.

//To output an IAM Role authentication ConfigMap from your Terraform configuration:

locals {
  config-map-aws-auth = <<CONFIGMAPAWSAUTH
apiVersion: v1
kind: ConfigMap
metadata:
  name: aws-auth
  namespace: kube-system
data:
  mapRoles: |
    - rolearn: ${aws_iam_role.EKSNodeRole.arn}
      username: system:node:{{EC2PrivateDNSName}}
      groups:
        - system:bootstrappers
        - system:nodes
CONFIGMAPAWSAUTH

}

output "config-map-aws-auth" {
  value = local.config-map-aws-auth
}

output "eks_rsa" {
  value = tls_private_key.eks_rsa.private_key_pem
}

