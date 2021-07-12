```sh
aws ec2 describe-images --owners amazon --filters "Name=name,Values=Windows_Server-2016-English*Containers*" --query 'sort_by(Images, &CreationDate)[].Name'
```

```sh
aws ec2 describe-images --owners amazon --filters "Name=name,Values=Windows_Server-2016-English*Containers*" --query 'sort_by(Images, &CreationDate)[].Name'
```
