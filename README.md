# Gohai

[![license](http://img.shields.io/badge/license-MIT-red.svg?style=flat)](http://kentaro.mit-license.org/)

Gohai is a tool which collects an inventory of system information. It aims to implement some parts of features from [facter](https://github.com/puppetlabs/facter) and [ohai](https://github.com/opscode/ohai).  It's forked from Kentaro Kuribayashi's [verity](https://github.com/kentaro/verity).

## Usage

Gohai will build and install with `go get`. We require at least Go 1.7.

```sh
go get github.com/DataDog/gohai
```

Running it will dump json formatted output:

```sh
$ gohai | jq .
{
  "cpu": {
    "cpu_cores": "2",
    "family": "6",
    "mhz": "2600",
    "model": "58",
    "model_name": "Intel(R) Core(TM) i5-3230M CPU @ 2.60GHz",
    "stepping": "9",
    "vendor_id": "GenuineIntel"
  },
  "filesystem": [
    {
      "kb_size": "244277768",
      "mounted_on": "/",
      "name": "/dev/disk0s2"
    }
  ],
  "memory": {
    "swap_total": "4096.00M",
    "total": "8589934592"
  },
  "network": {
    "ipaddress": "192.168.1.6",
    "ipaddressv6": "fe80::5626:96ff:fed3:5811",
    "macaddress": "54:26:96:d3:58:11"
  },
  "platform": {
    "GOOARCH": "amd64",
    "GOOS": "darwin",
    "goV": "1.2.1",
    "hostname": "new-host.home",
    "kernel_name": "Darwin",
    "kernel_release": "12.5.0",
    "kernel_version": "Darwin Kernel Version 12.5.0: Sun Sep 29 13:33:47 PDT 2013; root:xnu-2050.48.12~1/RELEASE_X86_64",
    "machine": "x86_64",
    "os": "Darwin",
    "processor": "i386",
    "pythonV": "2.7.2"
  }
}
```

Pipe it through eg. `jq` or `python -m json.tool` for pretty output.

## How to build

Just run `go build`!

## Build with version info

To build Gohai with version information, use `make.go`:

```sh
go run make.go
```

It will build gohai using the `go build` command, with the version info passed through `-ldflags`.

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
