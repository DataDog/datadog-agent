variable "AWS_ACCESS_KEY_ID" {}
variable "AWS_SECRET_ACCESS_KEY" {}
variable "AWS_REGION" {
  default = "eu-west-1"
}

variable "INSTANCE_USERNAME" {
  default = "gitlab"
}
variable "INSTANCE_PASSWORD" {
  default = "Bionic!"
}
variable "KEY_NAME" {
  default = "ServersBasicAccessKey1"
}
