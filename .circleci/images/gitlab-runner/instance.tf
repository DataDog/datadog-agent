resource "aws_instance" "gitlab-runner" {

  ami = "${lookup(var.WIN_AMIS, "base2016")}"
  instance_type = "t3.large"
  security_groups = ["winrms-open"]

  key_name = "${var.KEY_NAME}"

  root_block_device {
    volume_size = "200"
    volume_type = "standard"
  }

  tags {
    Name = "Windows Gitlab Runner"
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

provisioner "file" {
  source = "deployed.txt"
  destination = "C:/deployed.txt"
}

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


locals {
  Makefile = <<MAKEFILE
check:
	ansible windows -i hosts -m win_ping

provision:
        ansible_gitlab_runner.sh

MAKEFILE

  hostsfile = <<HOSTS
[windows]
${aws_instance.gitlab-runner.public_ip}

[windows:vars]
ansible_user=${var.INSTANCE_USERNAME}
ansible_password=${var.INSTANCE_PASSWORD}
ansible_connection=winrm
ansible_winrm_server_cert_validation=ignore

HOSTS
}



output "Makefile" {
  value = "${local.Makefile}"
}

output "hosts" {
  value = "${local.hostsfile}"
}
