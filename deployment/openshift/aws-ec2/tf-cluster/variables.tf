variable "public_key_path" {
  description = "Public key to use for SSH access"
  default = "./okd_rsa.pub"
}

variable "AWS_ACCESS_KEY_ID" {}
variable "AWS_SECRET_ACCESS_KEY" {}
variable "AWS_REGION" {
  description = "Region to deploy the cluster into"
  default = "eu-west-1"
}

variable "CLUSTER_NAME" {}
