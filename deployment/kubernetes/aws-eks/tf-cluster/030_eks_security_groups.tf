//EKS Master Cluster Security Group

//This security group controls networking access to the Kubernetes masters.
//Needs to be configured also with an ingress rule to allow traffic from the worker nodes.

resource "aws_security_group" "eks-control-plane-sg" {
  name        = "${var.CLUSTER_NAME}-control-plane"
  description = "Cluster communication with worker nodes [${var.CLUSTER_NAME}]"
  vpc_id      = aws_vpc.cluster.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# OPTIONAL: Allow inbound traffic from your local workstation external IP
#           to the Kubernetes. You will need to replace A.B.C.D below with
#           your real IP. Services like icanhazip.com can help you find this.
//resource "aws_security_group_rule" "eks-ingress-workstation-https" {
//  cidr_blocks       = ["A.B.C.D/32"]
//  description       = "Allow workstation to communicate with the cluster API Server"
//  from_port         = 443
//  protocol          = "tcp"
//  security_group_id = "${aws_security_group.eks-control-plane-sg.id}"
//  to_port           = 443
//  type              = "ingress"
//}
