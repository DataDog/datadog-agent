// create vpc

//Base VPC Networking
//EKS requires the usage of Virtual Private Cloud to provide the base for its networking configuration.

resource "aws_vpc" "cluster" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true

  tags = merge(
    local.common_tags,
    {
      "Name" = "${var.CLUSTER_NAME}-eks-vpc"
    },
  )
}

//The below will create a ${var.public_subnet_cidr} VPC,
//two ${var.public_subnet_cidr} public subnets,
//two ${var.private_subnet_cidr} private subnets with nat instances,
//an internet gateway,
//and setup the subnet routing to route external traffic through the internet gateway:

// public subnets
resource "aws_subnet" "eks-public" {
  vpc_id = aws_vpc.cluster.id

  cidr_block        = var.public_subnet_cidr
  availability_zone = local.availabilityzone

  tags = merge(
    local.common_tags,
    {
      "Name" = "${var.CLUSTER_NAME}-eks-public"
    },
  )
}

resource "aws_subnet" "eks-public-2" {
  vpc_id = aws_vpc.cluster.id

  cidr_block        = var.public_subnet_cidr2
  availability_zone = local.availabilityzone2

  tags = merge(
    local.common_tags,
    {
      "Name" = "${var.CLUSTER_NAME}-eks-public-2"
    },
  )
}

// private subnet
resource "aws_subnet" "eks-private" {
  vpc_id = aws_vpc.cluster.id

  cidr_block        = var.private_subnet_cidr
  availability_zone = local.availabilityzone

  tags = merge(
    local.common_tags,
    {
      "Name" = "${var.CLUSTER_NAME}-eks-private"
    },
  )
}

resource "aws_subnet" "eks-private-2" {
  vpc_id = aws_vpc.cluster.id

  cidr_block        = var.private_subnet_cidr2
  availability_zone = local.availabilityzone2

  tags = merge(
    local.common_tags,
    {
      "Name" = "${var.CLUSTER_NAME}-eks-private-2"
    },
  )
}

// internet gateway, note: creation takes a while

resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.cluster.id
  tags = {
    Environment = var.CLUSTER_NAME
  }
}

// reserve elastic ip for nat gateway

resource "aws_eip" "nat_eip" {
  vpc = true
  tags = {
    Environment = var.CLUSTER_NAME
  }
}

resource "aws_eip" "nat_eip_2" {
  vpc = true
  tags = {
    Environment = var.CLUSTER_NAME
  }
}

// create nat once internet gateway created
resource "aws_nat_gateway" "nat_gateway" {
  allocation_id = aws_eip.nat_eip.id
  subnet_id     = aws_subnet.eks-public.id
  depends_on    = [aws_internet_gateway.igw]
  tags = {
    Environment = var.CLUSTER_NAME
  }
}

resource "aws_nat_gateway" "nat_gateway_2" {
  allocation_id = aws_eip.nat_eip_2.id
  subnet_id     = aws_subnet.eks-public-2.id
  depends_on    = [aws_internet_gateway.igw]
  tags = {
    Environment = var.CLUSTER_NAME
  }
}

//Create private route table and the route to the internet
//This will allow all traffics from the private subnets to the internet through the NAT Gateway (Network Address Translation)

resource "aws_route_table" "private_route_table" {
  vpc_id = aws_vpc.cluster.id
  tags = {
    Environment = var.CLUSTER_NAME
    Name        = "${var.CLUSTER_NAME}-private-route-table"
  }
}

resource "aws_route_table" "private_route_table_2" {
  vpc_id = aws_vpc.cluster.id
  tags = {
    Environment = var.CLUSTER_NAME
    Name        = "${var.CLUSTER_NAME}-private-route-table-2"
  }
}

resource "aws_route" "private_route" {
  route_table_id         = aws_route_table.private_route_table.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.nat_gateway.id
}

resource "aws_route" "private_route_2" {
  route_table_id         = aws_route_table.private_route_table_2.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.nat_gateway_2.id
}

resource "aws_route_table" "eks-public" {
  vpc_id = aws_vpc.cluster.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }

  tags = {
    Environment = var.CLUSTER_NAME
    Name        = "${var.CLUSTER_NAME}-eks-public"
  }
}

resource "aws_route_table" "eks-public-2" {
  vpc_id = aws_vpc.cluster.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }

  tags = {
    Environment = var.CLUSTER_NAME
    Name        = "${var.CLUSTER_NAME}-eks-public-2"
  }
}

// associate route tables

resource "aws_route_table_association" "eks-public" {
  subnet_id      = aws_subnet.eks-public.id
  route_table_id = aws_route_table.eks-public.id
}

resource "aws_route_table_association" "eks-public-2" {
  subnet_id      = aws_subnet.eks-public-2.id
  route_table_id = aws_route_table.eks-public-2.id
}

resource "aws_route_table_association" "eks-private" {
  subnet_id      = aws_subnet.eks-private.id
  route_table_id = aws_route_table.private_route_table.id
}

resource "aws_route_table_association" "eks-private-2" {
  subnet_id      = aws_subnet.eks-private-2.id
  route_table_id = aws_route_table.private_route_table_2.id
}

