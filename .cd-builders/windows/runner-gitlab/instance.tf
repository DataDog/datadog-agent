resource "aws_security_group" "winrmopen" {
  name        = "gitlab-agent6-winrm-open"
  description = "Access for troubleshouting purposes"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port       = 5985
    to_port         = 5985
    protocol        = "tcp"
    cidr_blocks     = [var.support_network]
  }

  ingress {
    from_port       = 5986
    to_port         = 5986
    protocol        = "tcp"
    cidr_blocks     = [var.support_network]
  }

  ingress {
    from_port       = 22
    to_port         = 22
    protocol        = "tcp"
    cidr_blocks     = [var.support_network]
  }

  ingress {
    from_port       = 3389
    to_port         = 3389
    protocol        = "tcp"
    cidr_blocks     = [var.support_network]
  }

  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    cidr_blocks     = ["0.0.0.0/0"]
  }

}


resource "aws_instance" "windows_runner" {
  ami                         = data.aws_ami.ami.id
  associate_public_ip_address = "true"
  availability_zone           = data.aws_availability_zone.default.id

  credit_specification {
    cpu_credits = "standard"
  }

  instance_type           = "t3.large"
  key_name                = var.keyname

  root_block_device {
    delete_on_termination = "true"
    encrypted             = "false"
#    iops                  = "100"
    volume_size           = "150"
    volume_type           = "gp2"
  }

  subnet_id         = data.aws_subnet.default.id

  tags = {
    Environment = "gitlab"
    Name        = "STS BACKWARD AGENT6 win builder"
  }

  tenancy                = "default"
  vpc_security_group_ids = [aws_security_group.winrmopen.id]

  user_data = file("cloud-init.txt")

}
