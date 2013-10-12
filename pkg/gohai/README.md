# Verity

Verity is an inventory collector of a system. It aims to implement some parts of features from [facter](https://github.com/puppetlabs/facter) and/or [ohai](https://github.com/opscode/ohai).

## Usage

Install `verity` command by `go get`:

```
$ go get github.com/kentaro/verity
```

Then execute the command:

```
$ verity
{"cpu":{"cache_size":"6144 KB","model_name":"Intel(R) Core(TM) i7-2677M CPU @ 1.80GHz","processor":"0","stepping":"7","total":"1","vendor_id":"GenuineIntel"},"hostname":"localhost.localdomain","memory":{"active":"59832kB","anon_pages":"10672kB","bounce":"0kB","buffers":"14156kB","cached":"81084kB","commit_limit":"2856252kB","committed_as":"331420kB","dirty":"4kB","free":"316972kB","inactive":"46080kB","mapped":"7740kB","nfs_unstable":"0kB","page_tables":"1912kB","slab":"33700kB","slab_reclaimable":"13348kB","slab_unreclaim":"20352kB","swap_cached":"0kB","swap_free":"2621432kB","swap_total":"2621432kB","total":"469644kB","vmalloc_chunk":"34359711736kB","vmalloc_total":"34359738367kB","vmalloc_used":"20736kB","writeback":"0kB"}}
```

## How to Hack

This repository provides Linux development environment with Vagrant.

```
$ vagrant up
$ vagrant ssh
[vagrant $] cd home/vagrant/go/src/github.com/kentaro/verity
```

To build `verity` command:

```
[vagrant $] make
```

To test whole the project:

```
[vagrant $] make test
```

## See Also

  * [facter](https://github.com/puppetlabs/facter)
  * [ohai](https://github.com/opscode/ohai).

## Author

  * [Kentaro Kuribayashi](http://kentarok.org/)

## License

  * MIT http://kentaro.mit-license.org/
