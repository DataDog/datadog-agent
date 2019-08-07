//  Notes: We could make the internal domain a variable, but not sure it is
//  really necessary.

//  Create the internal DNS.
resource "aws_route53_zone" "internal" {
  name = "openshift.local"
  comment = "OpenShift Cluster Internal DNS"
  vpc {
    vpc_id = "${aws_vpc.openshift.id}"
  }
  tags {
    Name    = "OpenShift Internal DNS"
    Project = "openshift"
  }
}

//  Routes for 'master', 'node1' and 'node2'.
resource "aws_route53_record" "master-a-record" {
    zone_id = "${aws_route53_zone.internal.zone_id}"
    name = "master.openshift.local"
    type = "A"
    ttl  = 300
    records = [
        "${aws_instance.master.private_ip}"
    ]
}
resource "aws_route53_record" "node1-a-record" {
    zone_id = "${aws_route53_zone.internal.zone_id}"
    name = "node1.openshift.local"
    type = "A"
    ttl  = 300
    records = [
        "${aws_instance.node1.private_ip}"
    ]
}
resource "aws_route53_record" "node2-a-record" {
    zone_id = "${aws_route53_zone.internal.zone_id}"
    name = "node2.openshift.local"
    type = "A"
    ttl  = 300
    records = [
        "${aws_instance.node2.private_ip}"
    ]
}
