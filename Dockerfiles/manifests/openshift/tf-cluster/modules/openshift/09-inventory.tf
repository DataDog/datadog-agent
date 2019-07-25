//  Collect together all of the output variables needed to build to the final
//  inventory from the inventory template.
data "template_file" "inventory" {
  template = "${file("${path.cwd}/inventory.template.cfg")}"
  vars {
    access_key = "${var.aws_access_key}"
    secret_key = "${var.aws_secret_key}"
    public_hostname = "${aws_instance.master.public_ip}.xip.io"
    master_inventory = "${aws_instance.master.private_dns}"
    master_hostname = "${aws_instance.master.private_dns}"
    node1_hostname = "${aws_instance.node1.private_dns}"
    node2_hostname = "${aws_instance.node2.private_dns}"
    cluster_id = "${var.cluster_id}"
  }
}

//  Create the inventory.
resource "local_file" "inventory" {
  content     = "${data.template_file.inventory.rendered}"
  filename = "${path.cwd}/inventory.cfg"
}
