variable "AWS_ACCESS_KEY_ID" {}
variable "AWS_SECRET_ACCESS_KEY" {}
variable "AWS_REGION" {
  default = "eu-west-1"
}
variable "WIN_AMIS" {
  type = "map"
  default = {
    base2016 = "ami-0a5cbbecdba7dfe83"
    base2016docker = "ami-0ba0b60f053f0978e"
  }
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
