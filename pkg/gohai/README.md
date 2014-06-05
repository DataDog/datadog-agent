# Gohai

Verity is a tool/library to build an inventory of system information. It aims to implement some parts of features from [facter](https://github.com/puppetlabs/facter) and [ohai](https://github.com/opscode/ohai).

## Usage

Do `go get` from this repository:

```
$ go get github.com/DataDog/gohai
```

Then execute the commands:

```
$ go build github.com/DataDog/gohai
$ gohai
{"cpu":{"cpu_cores":"2","family":"6","mhz":"2600","model":"58","model_name":"Intel(R) Core(TM) i5-3230M CPU @ 2.60GHz","stepping":"9","vendor_id":"GenuineIntel"},"filesystem":[{"kb_size":"244277768","mounted_on":"/","name":"/dev/disk0s2"}],"memory":{"swap_total":"4096.00M","total":"8589934592"},"network":{"ipaddress":"192.168.1.6","ipaddressv6":"fe80::5626:96ff:fed3:5811","macaddress":"54:26:96:d3:58:11"},"platform":{"GOOARCH":"amd64","GOOS":"darwin","goV":"1.2.1","hostname":"new-host.home","kernel_name":"Darwin","kernel_release":"12.5.0","kernel_version":"Darwin Kernel Version 12.5.0: Sun Sep 29 13:33:47 PDT 2013; root:xnu-2050.48.12~1/RELEASE_X86_64","machine":"x86_64","os":"Darwin","processor":"i386","pythonV":"2.7.2"}}
```

## How to cross build

To build the binary file for several platforms, we use goxc
```
go get github.com/laher/goxc
goxc -bc='linux,darwin,windows' -d=[BUILD_DIR]
```

## See Also

  * [facter](https://github.com/puppetlabs/facter)
  * [ohai](https://github.com/opscode/ohai)

## Author

  * [Kentaro Kuribayashi](http://kentarok.org/)

## License

  * MIT http://kentaro.mit-license.org/
