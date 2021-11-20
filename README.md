# pyon-upload
Lightweight web server for uploading files written in Go, compatible with Pomf frontend using AWS S3 as a storage backend.

## Building
A binary can be built simply by running `go build cmd/pyon-upload/main.go`

By default pyon-upload expects to find a config file called `pyon-upload.conf` in `/etc/pyon-upload/`. The name and location of this config file can be changed by building with `go build -ldflags "-X main.configLocation=/path/to/config/example.conf" cmd/pyon-upload/main.go`.

## AWS Credentials
Pyon-upload will get the AWS credential id and key values from the environment variables `PYON_AWS_ACCESS_ID` and `PYON_AWS_ACCESS_KEY`. Alternatively, you can specify `id` and `key` variables under the `[aws]` section of the config file for a less secure configuration.

## Running
Before running you should modify the config file to match your environment. In particular you probably want to modify at least the `ssl_certificate`, `ssl_key`, `database_path`, `placeholder_dir`, `region` and `bucket` variables.

Simply running the binary will start the server, provided that a config file is present in the correct location, AWS credentials are set and no other process is already bound to the specified port.

Pyon-upload was developed and tested on a Linux environment. While it should be possible to run it on a Windows machine, this has not been tested and may have unexpected bugs.

If you are using Pomf as a frontend, you should replace `upload.php` in `pomf.min.js` with `https://example.org:16421/upload`, using the hostname and port you are running on instead of `example.org:16421`.

## Behavior
Pyon-upload will store files in the AWS S3 bucket specified in the config file given the correct credentials. It will also create an empty placeholder file with the same filename in the directory specified by the `placeholder_dir` variable in the config file. The existence of a placeholder file can be checked in an Nginx config through the following command:
```
if (-f /path/to/placeholder/dir/$uri)
```
This makes it possible to prevent requests to S3 for files that do not exist and makes it easy to remove public access to a file.

On the first run, pyon-upload will create a new sqlite3 database in the location specified by `database_path` and `database_filename`. Subsequent runs will reuse that database.

## Package
`build/package/arch` contains a PKGBUILD file which can be used to build pyon-upload as an Arch Linux package. This package can be built using the following commands:
```
git clone https://github.com/exolyte/pyon-upload
cd pyon-upload/build/package/arch
makepkg
```
The package also contains a simple systemd service file which can be used to start and stop pyon-upload with `systemctl start/stop pyon-upload.service` once the package is installed.