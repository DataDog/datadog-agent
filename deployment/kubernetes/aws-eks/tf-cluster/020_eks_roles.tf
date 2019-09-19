//Kubernetes Masters
//This is where the EKS service comes into play.
//It requires a few operator managed resources beforehand so that Kubernetes can properly manage
//other AWS services as well as allow inbound networking communication from your local workstation
//(if desired) and worker nodes.

//EKS Master Cluster IAM Role
//
//IAM role and policy to allow the EKS service to manage or retrieve data from other AWS services.
//For the latest required policy, see the EKS User Guide.

resource "aws_iam_role" "EKSClusterRole" {
  name               = "EKSClusterRole-${var.CLUSTER_NAME}"
  description        = "Allows EKS to manage clusters on your behalf."
  assume_role_policy = <<POLICY
{
   "Version":"2012-10-17",
   "Statement":[
      {
         "Effect":"Allow",
         "Principal":{
            "Service":"eks.amazonaws.com"
         },
         "Action":"sts:AssumeRole"
      }
   ]
}
POLICY

}

//https://docs.aws.amazon.com/eks/latest/userguide/service_IAM_role.html

resource "aws_iam_role_policy_attachment" "eks-policy-AmazonEKSClusterPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.EKSClusterRole.name
}

resource "aws_iam_role_policy_attachment" "eks-policy-AmazonEKSServicePolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSServicePolicy"
  role       = aws_iam_role.EKSClusterRole.name
}

