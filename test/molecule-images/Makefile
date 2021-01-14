create-win-ami:
	packer build packer-windows.json

# aws ec2 describe-images --owners amazon --filters "Name=name,Values=Windows_Server-2012-RTM-English-64Bit-Base*" --query 'sort_by(Images, &CreationDate)[].Name'
create-win12-ami:
	packer build packer-windows-2012.json

create-receiver-ami:
	packer build packer-receiver.json
