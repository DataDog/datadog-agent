
terraform {
  backend "s3" {
    bucket = "lupulus-terraform-state"
    key    = "aws-openshift.terraform.tfstate"
    region = "eu-west-1"
  }
}

//  Setup the core provider information.
provider "aws" {
  region  = "${var.AWS_REGION}"
  version = "~> 1.26"
}

//  Create the OpenShift cluster using our module.
module "openshift" {
  source          = "./modules/openshift"
  region          = "${var.AWS_REGION}"
  amisize         = "t2.large"    //  Smallest that meets the min specs for OS
  vpc_cidr        = "10.0.0.0/16"
  subnet_cidr     = "10.0.1.0/24"
  key_name        = "openshift"
  public_key_path = "${var.public_key_path}"
  cluster_name    = "${var.CLUSTER_NAME}-cluster"
  cluster_id      = "${var.CLUSTER_NAME}-cluster-${var.AWS_REGION}"
  aws_access_key  = "${var.AWS_ACCESS_KEY_ID}"
  aws_secret_key  = "${var.AWS_SECRET_ACCESS_KEY}"
}

//  Output some useful variables for quick SSH access etc.
output "master-url" {
  value = "https://${module.openshift.master-public_ip}.xip.io:8443"
}
output "master-public_ip" {
  value = "${module.openshift.master-public_ip}"
}
output "bastion-public_ip" {
  value = "${module.openshift.bastion-public_ip}"
}
