Getting repo up

/etc/yum.repos.d/myrepo.repo

[myrepo]
name=This is my repo
baseurl=https://repobase/master
enabled=1
gpgcheck=0

yum makecache --disablerepo=* --enablerepo=myrepo

Checking packages in repo

yum --disablerepo="*" --enablerepo="myrepo" list available
