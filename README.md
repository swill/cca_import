
cca_import
==========

A cross platform tool which allows you to specify a directory and a bucket on the Cloud.ca object store and the directory will be uploaded to the specified bucket.  If the bucket does not exist, it will be created before uploading.

The code is not required to use the tool, you can just grab the appropriate binary and use it.


USAGE
-----

The usage documentation for the tool is accessible through the `-h` or `-help` flags.

``` bash
$ ./cca_import -h
Usage of ./cca_import:
  -bucket="": The container where the files should be uploaded
  -concurrent=4: The number of files to be uploaded concurrently (reduce if 'too many files open' errors occur)
  -dir="": Absolute or relative path to a directory to be uploaded
  -endpoint="https://auth-east.cloud.ca/v2.0": The Cloud.ca object storage public url
  -identity="": Your Cloud.ca object storage identity
  -password="": Your Cloud.ca object storage password
  -prefix="": A prefix added to the path of each object uploaded to the bucket
```

An example run would look like the following.

```
$ ./cca_import -dir="/abs/or/rel/path/to/dir" -bucket="bucket_name" -identity="check_your_profile" -password="check_your_profile"
 uploaded: cca_import_darwin_386.zip
 uploaded: cca_import_darwin_amd64.zip
 uploaded: cca_import_dragonfly_386.zip
 uploaded: cca_import_dragonfly_amd64.zip
 uploaded: cca_import_freebsd_386.zip
 uploaded: cca_import_freebsd_amd64.zip
 uploaded: cca_import_freebsd_arm.zip
 uploaded: cca_import_linux_386.tar.gz
 uploaded: cca_import_linux_amd64.tar.gz
 uploaded: cca_import_linux_arm.tar.gz
 uploaded: cca_import_nacl_386.zip
 uploaded: cca_import_nacl_amd64p32.zip
 uploaded: cca_import_nacl_arm.zip
 uploaded: cca_import_netbsd_386.zip
 uploaded: cca_import_netbsd_amd64.zip
 uploaded: cca_import_netbsd_arm.zip
 uploaded: cca_import_openbsd_386.zip
 uploaded: cca_import_openbsd_amd64.zip
 uploaded: cca_import_plan9_386.zip
 uploaded: cca_import_snapshot_amd64.deb
 uploaded: cca_import_snapshot_armhf.deb
 uploaded: cca_import_snapshot_i386.deb
 uploaded: cca_import_solaris_amd64.zip
 uploaded: cca_import_windows_386.zip
 uploaded: cca_import_windows_amd64.zip
```


BUILDING FROM SOURCE
--------------------

If you want to run from source you would do the following.

``` bash
$ git clone https://github.com/swill/cca_import.git
$ cd cca_import
$ go build
$ ./cca_import -h
```


CROSS COMPILING
---------------

Using the tool from source is not ideal, instead it should be cross compiled and the executables should be distributed.  Since this is written in Go (golang), it will have to be compiled for each OS independently.  A convenience script `_build.sh` has been added to the project to simplify the cross compilation process.

Compilation process:
``` bash
$ cd /path/to/cca_import
$ ./_build.sh
```

