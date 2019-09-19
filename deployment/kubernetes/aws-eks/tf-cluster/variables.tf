variable "AWS_ACCESS_KEY_ID" {
}

variable "AWS_SECRET_ACCESS_KEY" {
}

variable "AWS_REGION" {
  default = "eu-west-1"
}

variable "SCALING_DESIRED_CAPACITY" {
  default = 2
}

variable "CLUSTER_NAME" {
}

locals {
  availabilityzone  = "${var.AWS_REGION}a"
  availabilityzone2 = "${var.AWS_REGION}b"

  cluster_name = "${var.CLUSTER_NAME}-cluster"

  //  NOTE: The usage of the specific kubernetes.io/cluster/*
  //  resource tags below are required for EKS and Kubernetes to discover
  //  and manage networking resources.

  common_tags = {
    "Environment"                                 = var.CLUSTER_NAME
    "kubernetes.io/cluster/${local.cluster_name}" = "shared"
  }
}

variable "vpc_cidr" {
  description = "CIDR for the whole VPC"
  default     = "10.11.0.0/16"
}

// Primary pair of public/private networks

variable "public_subnet_cidr" {
  description = "CIDR for the Public Subnet"
  default     = "10.11.0.0/24"
}

variable "private_subnet_cidr" {
  description = "CIDR for the Private Subnet"
  default     = "10.11.1.0/24"
}

// Secondary pair of public/private networks (if you ever needed that)

variable "public_subnet_cidr2" {
  description = "CIDR for the Public Subnet"
  default     = "10.11.2.0/24"
}

variable "private_subnet_cidr2" {
  description = "CIDR for the Private Subnet"
  default     = "10.11.3.0/24"
}

