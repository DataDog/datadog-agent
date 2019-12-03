set -x

# Elevate privileges, retaining the environment.
sudo -E su
# Install dev tools.
#install epel
yum install -y epel-release
yum install -y NetworkManager
systemctl | grep "NetworkManager.*running" 
if [ $? -eq 1 ]; then
	systemctl start NetworkManager
	systemctl enable NetworkManager
fi
setenforce 0
sed -i 's/SELINUX=enforcing/SELINUX=permissive/g' /etc/selinux/config
yum install -y centos-release-openshift-origin311
# install the packages for Ansible
yum  install -y pyOpenSSL
