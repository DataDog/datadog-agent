# Gohai

[![license](http://img.shields.io/badge/license-MIT-red.svg?style=flat)](http://kentaro.mit-license.org/)

Gohai is a tool which collects an inventory of system information. It aims to implement some parts of features from [facter](https://github.com/puppetlabs/facter) and [ohai](https://github.com/opscode/ohai).  It's forked from Kentaro Kuribayashi's [verity](https://github.com/kentaro/verity).

## Updating CPU Information

Some information about CPUs is drawn from the source of the `util-linux` utility `lscpu`.
To update this information, such as when new processors are released, run

```
python cpu/from-lscpu-arm.py /path/to/lscpu-arm.c > cpu/lscpu_linux_arm64.go
```

## See Also

  * [facter](https://github.com/puppetlabs/facter)
  * [ohai](https://github.com/opscode/ohai)

## Author

  * [Kentaro Kuribayashi](http://kentarok.org/)

## License

  * MIT http://kentaro.mit-license.org/
