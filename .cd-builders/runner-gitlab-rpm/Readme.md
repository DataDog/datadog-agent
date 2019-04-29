
In order to add a my-app-1.0.0.x86_64.rpm package to a yum repo hosted in the yummy-yummy S3 bucket, at the path centos/6:

rpm-s3 -b yummy-yummy -p "centos/6" my-app-1.0.0.x86_64.rpm
