resource "aws_vpc" "app_vpc" {
  cidr_block = "10.10.0.0/16"
  assign_generated_ipv6_cidr_block = false
  enable_dns_support = true
  tags {
    Name = "${local.readable_env_name}"
    env = "${local.env}"
  }
}


# create subnets

resource "aws_subnet" "pub_subnet1"{
  # Ensures subnet is created in it's own AZ
  availability_zone = "${data.aws_availability_zones.available.names[0]}"
  vpc_id = "${aws_vpc.app_vpc.id}"
  cidr_block = "10.10.1.0/24"
  tags {
    Name = "${local.readable_env_name}-sb-pub-${local.region}-${data.aws_availability_zones.available.names[0]}"
    env = "${local.env}"
  }
}

resource "aws_subnet" "pub_subnet2"{
  # Ensures subnet is created in it's own AZ
  availability_zone = "${data.aws_availability_zones.available.names[1]}"
  vpc_id = "${aws_vpc.app_vpc.id}"
  cidr_block = "10.10.2.0/24"
  tags {
    Name = "${local.readable_env_name}-sb-pub-${local.region}-${data.aws_availability_zones.available.names[1]}"
    env = "${local.env}"
  }
}

resource "aws_subnet" "priv_subnet1"{
  # Ensures subnet is created in it's own AZ
  availability_zone = "${data.aws_availability_zones.available.names[0]}"
  vpc_id = "${aws_vpc.app_vpc.id}"
  cidr_block = "10.10.3.0/24"
  tags {
    Name = "${local.readable_env_name}-sb-priv-${local.region}-${data.aws_availability_zones.available.names[0]}"
    env = "${local.env}"
  }
}

resource "aws_subnet" "priv_subnet2"{
  # Ensures subnet is created in it's own AZ
  availability_zone = "${data.aws_availability_zones.available.names[2]}"
  vpc_id = "${aws_vpc.app_vpc.id}"
  cidr_block = "10.10.4.0/24"
  tags {
    Name = "${local.readable_env_name}-sb-priv-${local.region}-${data.aws_availability_zones.available.names[1]}"
    env = "${local.env}"
  }
}

# /create subnets

resource "aws_internet_gateway" "app_igw" {
  vpc_id = "${aws_vpc.app_vpc.id}"
  tags {
    Name = "${local.env}-igw"
    env = "${local.env}"
  }
}


# routes

resource "aws_default_route_table" "default" {
  default_route_table_id = "${aws_vpc.app_vpc.default_route_table_id}"

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = "${aws_internet_gateway.app_igw.id}"
  }

  tags {
    Name = "${local.readable_env_name}-route-main"
    env = "${local.env}"
  }

}

# / routes
