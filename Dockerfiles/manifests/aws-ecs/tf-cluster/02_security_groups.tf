resource "aws_security_group" "vpc_security_groups_elb" {
  name = "${local.readable_env_name}-public-ELB"
  description = "public access"
  vpc_id = "${aws_vpc.app_vpc.id}"

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Internal HTTPS access from anywhere
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags {
    Name = "${local.readable_env_name}-public-ELB"
    env = "${local.env}"
  }


}

resource "aws_security_group" "vpc_security_groups_bastion" {
  name = "${local.readable_env_name}-public-BASTION"
  description = "public access"
  vpc_id = "${aws_vpc.app_vpc.id}"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags {
    Name = "${local.readable_env_name}-public-BASTION"
    env = "${local.env}"
  }


}

resource "aws_security_group" "vpc_security_groups_cluster" {
  name = "${local.readable_env_name}-private-CLUSTER"
  description = "ECS Cluster"
  vpc_id = "${aws_vpc.app_vpc.id}"

  # # FOR DEBUG PURPOSES ONLY, ELIMINATE ON PROD
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    security_groups = ["${aws_security_group.vpc_security_groups_bastion.id}"]
  }

  ingress {
    from_port   = 0
    to_port     = 65535
    protocol    = "tcp"
    security_groups = ["${aws_security_group.vpc_security_groups_elb.id}"]
  }


  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags {
    Name = "${local.readable_env_name}-private-CLUSTER"
    env = "${local.env}"
  }


}

resource "aws_security_group" "vpc_security_groups_datalayer" {
  name = "${local.readable_env_name}-private-DATALAYER"
  description = "Private data layer - RDS, elasticache, mongo, etc"
  vpc_id = "${aws_vpc.app_vpc.id}"

  # SPECIFY INGRESS RULES

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags {
    Name = "${local.readable_env_name}-private-DATALAYER"
    env = "${local.env}"
  }


}
