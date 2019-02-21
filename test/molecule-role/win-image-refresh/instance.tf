data "aws_ami" "app_server_ami" {
  most_recent = true
  owners      = ["801119661308"] # Amazon

  filter {
    name = "name"
    values = ["Windows_Server-2016-English-Full-Base-*"]
    #    values = ["ubuntu/images/hvm-ssd/ubuntu-bionic-18.04-amd64-server-*"]
  }

  filter {
    name = "virtualization-type"
    values = ["hvm"]
  }

  filter {
    name = "root-device-type"
    values = ["ebs"]
  }

}


resource "aws_instance" "molecule-runner" {

  ami = "${data.aws_ami.app_server_ami.image_id}"
  instance_type = "t3.large"
  security_groups = ["winrms-open"]

  key_name = "${var.KEY_NAME}"

  root_block_device {
    volume_size = "30"
    volume_type = "standard"
  }

  tags {
    Name = "Windows Moleculer"
  }
  user_data = <<EOF
<powershell>
net user ${var.INSTANCE_USERNAME} '${var.INSTANCE_PASSWORD}' /add /y
net localgroup administrators ${var.INSTANCE_USERNAME} /add

# Disable Complex Passwords
# Reference: http://vlasenko.org/2011/04/27/removing-password-complexity-requirements-from-windows-server-2008-core/
$seccfg = [IO.Path]::GetTempFileName()
secedit /export /cfg $seccfg
(Get-Content $seccfg) | Foreach-Object {$_ -replace "PasswordComplexity\s*=\s*1", "PasswordComplexity=0"} | Set-Content $seccfg
secedit /configure /db $env:windir\security\new.sdb /cfg $seccfg /areas SECURITYPOLICY
del $seccfg
Write-Host "Complex Passwords have been disabled." -ForegroundColor Green

# Disable Internet Explorer Security
# http://stackoverflow.com/a/9368555/2067999
$AdminKey = "HKLM:\SOFTWARE\Microsoft\Active Setup\Installed Components\{A509B1A7-37EF-4b3f-8CFC-4F3A74704073}"
$UserKey = "HKLM:\SOFTWARE\Microsoft\Active Setup\Installed Components\{A509B1A8-37EF-4b3f-8CFC-4F3A74704073}"
Set-ItemProperty -Path $AdminKey -Name "IsInstalled" -Value 0
Set-ItemProperty -Path $UserKey -Name "IsInstalled" -Value 0

Set-ExecutionPolicy Bypass -Scope Process -Force;
iex ((New-Object System.Net.WebClient).DownloadString('https://raw.githubusercontent.com/softasap/sa-win/master/bootstrap.ps1'))

</powershell>
EOF


connection {
  type = "winrm"
  timeout = "6m"
  user = "${var.INSTANCE_USERNAME}"
  password = "${var.INSTANCE_PASSWORD}"
  https = true
  insecure = true
  port = 5986
  }

}


resource "aws_ami_from_instance" "ami" {
  name               = "sts-windows-molecule"
  source_instance_id = "${aws_instance.molecule-runner.id}"
}

resource "null_resource" "ami_creator" {
  triggers {
    cluster_instance_ids = "${join(",", aws_instance.molecule-runner.*.id)}"
  }

  provisioner "local-exec" {
    command = "sleep 400 && aws ec2 copy-image --source-image-id ${aws_ami_from_instance.ami.id} --source-region ${var.AWS_REGION} --name sts-windows-molecule-release"
  }

  depends_on = ["aws_ami_from_instance.ami"]
}


data "aws_ami" "FinalAmi" {
  most_recent = true
  owners      = ["self"]

  filter {
    name = "name"
    values = ["sts-windows-molecule-release"]
  }

  filter {
    name = "virtualization-type"
    values = ["hvm"]
  }

  filter {
    name = "root-device-type"
    values = ["ebs"]
  }
  depends_on = ["null_resource.ami_creator"]
}

output "molecule_win_ami" {
  value = "${data.aws_ami.FinalAmi.image_id}"
}
