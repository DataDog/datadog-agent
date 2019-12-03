//  Create a role which OpenShift instances will assume.
//  This role has a policy saying it can be assumed by ec2
//  instances.
resource "aws_iam_role" "openshift-instance-role" {
  name = "openshift-instance-role"

  assume_role_policy = <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": "sts:AssumeRole",
            "Principal": {
                "Service": "ec2.amazonaws.com"
            },
            "Effect": "Allow",
            "Sid": ""
        }
    ]
}
EOF
}

//  This policy allows an instance to forward logs to CloudWatch, and
//  create the Log Stream or Log Group if it doesn't exist.
resource "aws_iam_policy" "openshift-policy-forward-logs" {
  name        = "openshift-instance-forward-logs"
  path        = "/"
  description = "Allows an instance to forward logs to CloudWatch"

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents",
        "logs:DescribeLogStreams"
      ],
      "Resource": [
        "arn:aws:logs:*:*:*"
      ]
    }
  ]
}
EOF
}

//  Attach the policies to the roles.
resource "aws_iam_policy_attachment" "openshift-attachment-forward-logs" {
  name       = "openshift-attachment-forward-logs"
  roles      = ["${aws_iam_role.openshift-instance-role.name}"]
  policy_arn = "${aws_iam_policy.openshift-policy-forward-logs.arn}"
}

//  Create a instance profile for the role.
resource "aws_iam_instance_profile" "openshift-instance-profile" {
  name  = "openshift-instance-profile"
  role = "${aws_iam_role.openshift-instance-role.name}"
}

//  Create a instance profile for the bastion. All profiles need a role, so use
//  our simple openshift instance role.
resource "aws_iam_instance_profile" "bastion-instance-profile" {
  name  = "bastion-instance-profile"
  role = "${aws_iam_role.openshift-instance-role.name}"
}

