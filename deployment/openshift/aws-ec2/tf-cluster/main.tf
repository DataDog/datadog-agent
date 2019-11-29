terraform {
  backend "s3" {
    bucket = "lupulus-sandbox-terraform-state"
    key    = "openshift-lupulus-cluster.terraform.tfstate"
    region = "eu-west-1"
  }
}

# Setup our providers so that we have deterministic dependecy resolution. 
provider "aws" {
  region  = var.AWS_REGION
  version = "~> 2.19"
}

provider "local" {
  version = "~> 1.3"
}

provider "template" {
  version = "~> 2.1"
}

//  Create the OpenShift cluster using our module.
module "openshift" {
  source          = "./modules/openshift"
  region          = var.AWS_REGION
  amisize         = "t2.large" //  Smallest that meets the min specs for OS
  centos_ami      = "ami-3548444c"
  vpc_cidr        = "10.0.0.0/16"
  subnet_cidr     = "10.0.1.0/24"
  key_name        = "openshift"
  public_key_path = var.public_key_path
  cluster_name    = "${var.CLUSTER_NAME}-cluster"
  cluster_id      = "${var.CLUSTER_NAME}-cluster-${var.AWS_REGION}"
  aws_access_key  = var.AWS_ACCESS_KEY_ID
  aws_secret_key  = var.AWS_SECRET_ACCESS_KEY
}

//  Output some useful variables for quick SSH access etc.
output "master-url" {
  value = "https://${module.openshift.master-public_ip}.xip.io:8443"
}

output "master-public_ip" {
  value = module.openshift.master-public_ip
}

output "bastion-public_ip" {
  value = module.openshift.bastion-public_ip
}

