//  Create an SSH keypair
resource "aws_key_pair" "keypair" {
  key_name   = "${var.key_name}"
  public_key = "${file(var.public_key_path)}"
}

//  Create the master userdata script.
data "template_file" "setup-master" {
  template = "${file("${path.module}/files/setup-master.sh")}"
  vars {
    region            = "${var.region}}"
    availability_zone = "${data.aws_availability_zones.azs.names[0]}"
  }
}

//  Launch configuration for the consul cluster auto-scaling group.
resource "aws_instance" "master" {
  ami                  = "${data.aws_ami.rhel7_5.id}"
  # Master nodes require at least 16GB of memory.
  instance_type        = "m4.xlarge"
  subnet_id            = "${aws_subnet.public-subnet.id}"
  user_data            = "${data.template_file.setup-master.rendered}"

  vpc_security_group_ids = [
    "${aws_security_group.openshift-vpc.id}",
    "${aws_security_group.openshift-public-ingress.id}",
    "${aws_security_group.openshift-public-egress.id}",
  ]

  //  We need at least 30GB for OpenShift, let's be greedy...
  root_block_device {
    volume_size = 50
    volume_type = "gp2"
  }

  # Storage for Docker, see:
  # https://docs.openshift.org/latest/install_config/install/host_preparation.html#configuring-docker-storage
  ebs_block_device {
    device_name = "/dev/sdf"
    volume_size = 80
    volume_type = "gp2"
  }

  key_name = "${aws_key_pair.keypair.key_name}"

  //  Use our common tags and add a specific name.
  tags = "${merge(
    local.common_tags,
    map(
      "Name", "OpenShift Master"
    )
  )}"
}

//  Create the node userdata script.
data "template_file" "setup-node" {
  template = "${file("${path.module}/files/setup-node.sh")}"
  vars {
    region            = "${var.region}}"
    availability_zone = "${data.aws_availability_zones.azs.names[0]}"
  }
}

//  Create the two nodes. This would be better as a Launch Configuration and
//  autoscaling group, but I'm keeping it simple...
resource "aws_instance" "node1" {
  ami                  = "${data.aws_ami.rhel7_5.id}"
  instance_type        = "${var.amisize}"
  subnet_id            = "${aws_subnet.public-subnet.id}"
  user_data            = "${data.template_file.setup-node.rendered}"

  vpc_security_group_ids = [
    "${aws_security_group.openshift-vpc.id}",
    "${aws_security_group.openshift-public-ingress.id}",
    "${aws_security_group.openshift-public-egress.id}",
  ]

  //  We need at least 30GB for OpenShift, let's be greedy...
  root_block_device {
    volume_size = 50
    volume_type = "gp2"
  }

  # Storage for Docker, see:
  # https://docs.openshift.org/latest/install_config/install/host_preparation.html#configuring-docker-storage
  ebs_block_device {
    device_name = "/dev/sdf"
    volume_size = 80
    volume_type = "gp2"
  }

  key_name = "${aws_key_pair.keypair.key_name}"

  //  Use our common tags and add a specific name.
  tags = "${merge(
    local.common_tags,
    map(
      "Name", "OpenShift Node 1"
    )
  )}"
}

resource "aws_instance" "node2" {
  ami                  = "${data.aws_ami.rhel7_5.id}"
  instance_type        = "${var.amisize}"
  subnet_id            = "${aws_subnet.public-subnet.id}"
  user_data            = "${data.template_file.setup-node.rendered}"

  vpc_security_group_ids = [
    "${aws_security_group.openshift-vpc.id}",
    "${aws_security_group.openshift-public-ingress.id}",
    "${aws_security_group.openshift-public-egress.id}",
  ]

  //  We need at least 30GB for OpenShift, let's be greedy...
  root_block_device {
    volume_size = 50
    volume_type = "gp2"
  }

  # Storage for Docker, see:
  # https://docs.openshift.org/latest/install_config/install/host_preparation.html#configuring-docker-storage
  ebs_block_device {
    device_name = "/dev/sdf"
    volume_size = 80
    volume_type = "gp2"
  }

  key_name = "${aws_key_pair.keypair.key_name}"

  //  Use our common tags and add a specific name.
  tags = "${merge(
    local.common_tags,
    map(
      "Name", "OpenShift Node 2"
    )
  )}"
}
