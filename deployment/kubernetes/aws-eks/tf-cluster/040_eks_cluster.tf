//
//EKS Master Cluster
//This resource is the actual Kubernetes master cluster. It can take a few minutes to provision in AWS.

resource "aws_eks_cluster" "eks-cluster" {
  name     = local.cluster_name
  role_arn = aws_iam_role.EKSClusterRole.arn
  version  = "1.12"

  vpc_config {
    security_group_ids = [aws_security_group.eks-control-plane-sg.id]

    subnet_ids = [
      aws_subnet.eks-private.id,
      aws_subnet.eks-private-2.id,
    ]
  }

  depends_on = [
    aws_iam_role_policy_attachment.eks-policy-AmazonEKSClusterPolicy,
    aws_iam_role_policy_attachment.eks-policy-AmazonEKSServicePolicy,
  ]
}

locals {
  kubeconfig = <<KUBECONFIG
apiVersion: v1
clusters:
- cluster:
    server: ${aws_eks_cluster.eks-cluster.endpoint}
    certificate-authority-data: ${aws_eks_cluster.eks-cluster.certificate_authority[0].data}
  name: kubernetes
contexts:
- context:
    cluster: kubernetes
    user: aws
  name: aws-${var.CLUSTER_NAME}
current-context: aws-${var.CLUSTER_NAME}
kind: Config
preferences: {}
users:
- name: aws
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1alpha1
      command: heptio-authenticator-aws
      args:
        - "token"
        - "-i"
        - "${local.cluster_name}"
KUBECONFIG

}

output "kubeconfig" {
  value = local.kubeconfig
}

