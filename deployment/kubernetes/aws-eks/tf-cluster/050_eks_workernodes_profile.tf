//Kubernetes Worker Nodes
//The EKS service does not currently provide managed resources for running worker nodes.
//Here we will create a few operator managed resources so that Kubernetes can properly manage
//other AWS services, networking access, and finally a configuration that allows
//automatic scaling of worker nodes.

//Worker Node IAM Role and Instance Profile
//IAM role and policy to allow the worker nodes to manage or retrieve data from other AWS services.
//It is used by Kubernetes to allow worker nodes to join the cluster.
//
//For the latest required policy, see the EKS User Guide.

resource "aws_iam_role" "EKSNodeRole" {
  name = "eks-${local.cluster_name}-node-role"

  assume_role_policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
POLICY

}

resource "aws_iam_role_policy_attachment" "eks-node-AmazonEKSWorkerNodePolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.EKSNodeRole.name
}

resource "aws_iam_role_policy_attachment" "eks-node-AmazonEKS_CNI_Policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.EKSNodeRole.name
}

resource "aws_iam_role_policy_attachment" "eks-node-AmazonEC2ContainerRegistryReadOnly" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.EKSNodeRole.name
}

resource "aws_iam_instance_profile" "eks-node-instance-profile" {
  name = "${var.CLUSTER_NAME}-instance-profile"
  role = aws_iam_role.EKSNodeRole.name
}

//Worker Node Security Group
//This security group controls networking access to the Kubernetes worker nodes.

resource "aws_security_group" "eks-nodes-sg" {
  name        = "${local.cluster_name}-nodes-sg"
  description = "Security group for all nodes in the cluster [${var.CLUSTER_NAME}] "
  vpc_id      = aws_vpc.cluster.id

  //    ingress {
  //      from_port       = 0
  //      to_port         = 0
  //      protocol        = "-1"
  //      description = "allow nodes to communicate with each other"
  //      self = true
  //    }

  //    ingress {
  //      from_port       = 1025
  //      to_port         = 65535
  //      protocol        = "tcp"
  //      description = "Allow worker Kubelets and pods to receive communication from the cluster control plane"
  //      security_groups = ["${aws_security_group.eks-control-plane.id}"]
  //    }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    "Name"                                        = "${local.cluster_name}-nodes-sg"
    "kubernetes.io/cluster/${local.cluster_name}" = "owned"
  }
}

//Worker Node Access to EKS Master Cluster
//Now that we have a way to know where traffic from the worker nodes is coming from,
//we can allow the worker nodes networking access to the EKS master cluster.

resource "aws_security_group_rule" "https_nodes_to_plane" {
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks-control-plane-sg.id
  source_security_group_id = aws_security_group.eks-nodes-sg.id
  depends_on = [
    aws_security_group.eks-nodes-sg,
    aws_security_group.eks-control-plane-sg,
  ]
}

resource "aws_security_group_rule" "communication_plane_to_nodes" {
  type                     = "ingress"
  from_port                = 1025
  to_port                  = 65534
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks-nodes-sg.id
  source_security_group_id = aws_security_group.eks-control-plane-sg.id
  depends_on = [
    aws_security_group.eks-nodes-sg,
    aws_security_group.eks-control-plane-sg,
  ]
}

resource "aws_security_group_rule" "nodes_internode_communications" {
  type              = "ingress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  description       = "allow nodes to communicate with each other"
  security_group_id = aws_security_group.eks-nodes-sg.id
  self              = true
}

