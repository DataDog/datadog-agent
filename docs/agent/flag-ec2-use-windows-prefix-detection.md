# Windows EC2 hostname starting with `EC2AMAZ-`

## Description of the issue

In v6 and v7 Agents, the default agent in-app hostname for Unix platform on EC2 is the instance-id. 
For Windows hosts, the default agent in-app hostname is the operating system hostname that starts with `EC2AMAZ-`.

Starting with v6.18.0 and v7.18.0, the Agent logs the following warning for Windows host on EC2 where the hostname starts with `EC2AMAZ-`.
`You may want to use the EC2 instance-id for the in-app hostname. For more information: https://dtdg.co/ec2-use-win-prefix-detection`

If this warning is logged, you can either:

* if you are satisfied with the in-app hostname, do nothing; or
* if you are not satisfied with the in-app hostname and want to use the instance-id, follow the instructions below

## Use EC2 instance-id for Windows host on EC2

Starting with Agent v6.15.0 and v7.15.0, the Agent supports the config option `ec2_use_windows_prefix_detection` (default: `false`). When set to `true`, the in-app hostname for Windows EC2 host is the instance-id:

* for new hosts, enabling this option will work immediately
* for hosts that already report to Datadog, after enabling this option, please contact our support team at support@datadoghq.com so that the in-app hostname can be changed to the EC2 instance-id.
