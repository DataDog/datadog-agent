data "aws_ami" "ecs_ami" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amzn-ami-*-amazon-ecs-optimized"]
  }
}

data "template_file" "user_data" {
  template = "${file("${path.module}/files/default-user-data.sh")}"

  vars {
    ecs_cluster_name = "${local.ecs_cluster_name}"
  }
}

data "aws_availability_zones" "available" {}