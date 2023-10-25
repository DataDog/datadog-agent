name "msodbcsql"
default_version "18-18.3.2.1-1"

build do
    command "curl https://packages.microsoft.com/keys/microsoft.asc | sudo tee /etc/apt/trusted.gpg.d/microsoft.asc"
    command "curl https://packages.microsoft.com/config/ubuntu/$(lsb_release -rs)/prod.list | sudo tee /etc/apt/sources.list.d/mssql-release.list"

    command "sudo apt-get update"
    command "sudo ACCEPT_EULA=Y apt-get install -y msodbcsql18"
end