# Packer script to build devenv image on GCP.
# 
# Usage:
#   packer init gcloud.pkr.hcl
#   packer build gcloud.pkr.hcl

packer {
  required_plugins {
    googlecompute = {
      version = ">= 0.0.1"
      source = "github.com/hashicorp/googlecompute"
    }
  }
}

source "googlecompute" "datadog-agent-windows-dev" {
  project_id = 
  network = 
  subnetwork = 

  source_image_family = "windows-2019"
  zone = "europe-west1-b"
  disk_size = 50
  machine_type = "e2-standard-4"
  communicator = "winrm"
  winrm_username = "packer_user"
  winrm_insecure = true
  winrm_use_ssl = true
  image_name = "agent-windows-dev-{{timestamp}}"
  image_family = "agent-windows-dev"
  metadata = {
    windows-startup-script-cmd = "winrm quickconfig -quiet & net user /add packer_user & net localgroup administrators packer_user /add & winrm set winrm/config/service/auth @{Basic=\"true\"}"
  }
}

build {
  sources = ["sources.googlecompute.datadog-agent-windows-dev"]
  provisioner "powershell" {
    scripts = [
      "scripts/Install-DevEnv.ps1",
      "scripts/Disable-WinRM.ps1"
    ]
  }
}

