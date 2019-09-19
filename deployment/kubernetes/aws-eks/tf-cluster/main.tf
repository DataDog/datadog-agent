//EKS Cluster: AWS managed Kubernetes cluster
//AutoScaling Group containing 2 t2.medium instances based on the latest EKS Amazon Linux 2
//AMI: Operator managed Kuberneted worker nodes for running Kubernetes service deployments
//Associated VPC, Internet Gateway, Security Groups, and Subnets:
//Operator managed networking resources for the EKS Cluster and worker node instances
//Associated IAM Roles and Policies:
//Operator managed access resources for EKS and worker node instances

// Remote state in S3 bucket
terraform {
  backend "s3" {
    bucket = "lupulus-sandbox-terraform-state"
    key    = "aws-eks.terraform.tfstate"
    region = "eu-west-1"
  }
}

// AWS setup
provider "aws" {
  version = "~> 2.0"
  region  = var.AWS_REGION
}

