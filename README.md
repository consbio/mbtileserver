# mbtileserver

A simple Go-based server for map tiles stored in [mbtiles](https://github.com/mapbox/mbtiles-spec)
format.

[![Build Status](https://travis-ci.org/consbio/mbtileserver.svg?branch=master)](https://travis-ci.org/consbio/mbtileserver)
[![Coverage Status](https://coveralls.io/repos/github/consbio/mbtileserver/badge.svg?branch=master)](https://coveralls.io/github/consbio/mbtileserver?branch=master)
[![GoDoc](https://godoc.org/github.com/consbio/mbtileserver?status.svg)](http://godoc.org/github.com/consbio/mbtileserver)
[![Go Report Card](https://goreportcard.com/badge/github.com/consbio/mbtileserver)](https://goreportcard.com/report/github.com/consbio/mbtileserver)

_Requires Go 1.10+._

It currently provides support for `png`, `jpg`, and `pbf` (vector tile)
tilesets according to version 1.0 of the mbtiles specification. Tiles
are served following the XYZ tile scheme, based on the Web Mercator
coordinate reference system. UTF8 Grids are also supported.

In addition to tile-level access, it provides:

-   TileJSON 2.1.0 endpoint for each tileset, with full metadata
    from the mbtiles file.
-   a preview map for exploring each tileset.
-   a minimal ArcGIS tile map service API (work in progress)

We have been able to host a bunch of tilesets on an
[AWS t2.nano](https://aws.amazon.com/about-aws/whats-new/2015/12/introducing-t2-nano-the-smallest-lowest-cost-amazon-ec2-instance/)
virtual machine without any issues.

## Goals

-   Provide a web tile API for map tiles stored in mbtiles format
-   Be fast
-   Run on small resource cloud hosted machines (limited memory & CPU)
-   Be easy to install and operate

## Installation

You can install this project with

```sh
go get github.com/consbio/mbtileserver
```

This will create and install an executable called `mbtileserver`.

## Usage

From within the repository root ($GOPATH/bin needs to be in in your $PATH):

```
$  mbtileserver --help
Serve tiles from mbtiles files.

Usage:
  mbtileserver [flags]

Flags:
  -c, --cert string       X.509 TLS certificate filename.  If present, will be used to enable SSL on the server.
  -d, --dir string        Directory containing mbtiles files. (default "./tilesets")
      --domain string     Domain name of this server
      --dsn string        Sentry DSN
  -h, --help              help for mbtileserver
  -k, --key string        TLS private key
      --path string       URL root path of this server (if behind a proxy)
  -p, --port int          Server port. (default 8000)
  -s, --secret-key string Shared secret key used for HMAC authentication
  -t, --tls               Auto TLS using Let's Encrypt
  -r, --redirect          Redirect HTTP to HTTPS
      --enable-reload     Enable graceful reload
  -v, --verbose           Verbose logging
```

So hosting tiles is as easy as putting your mbtiles files in the `tilesets`
directory and starting the server. Woo hoo!

You can have multiple directories in your `tilesets` directory; these will be converted into appropriate URLs:

`mytiles/foo/bar/baz.mbtiles` will be available at `/services/foo/bar/baz`.

When you want to remove, modify, or add new tilesets, simply restart the server process.

If a valid Sentry DSN is provided, warnings, errors, fatal errors, and panics will be reported to Sentry.

If `redirect` option is provided, the server also listens on port 80 and redirects to port 443.

If the `--tls` option is provided, the Let's Encrypt Terms of Service are accepted automatically on your behalf. Please review them [here](https://letsencrypt.org/repository/). Certificates are cached in a `.certs` folder created where you are executing `mbtileserver`. Please make sure this folder can be written by the `mbtileserver` process or you will get errors.

You can also set up server config using environment variables instead of flags, which may be more helpful when deploying in a docker image. Use the associated flag to determine usage. The following variables are available:

-   `PORT` (`--port`)
-   `TILE_DIR` (`--dir`)
-   `PATH_PREFIX` (`--path`)
-   `DOMAIN` (`--domain`)
-   `TLS_CERT` (`--cert`)
-   `TLS_PRIVATE_KEY` (`--key`)
-   `AUTO_TLS` (`--tls`)
-   `REDIRECT` (`--redirect`)
-   `DSN` (`--dsn`)
-   `VERBOSE` (`--verbose`)
-   `HMAC_SECRET_KEY` (`--secret-key`)

Example:

```
$ PORT=7777 TILE_DIR=./path/to/your/tiles VERBOSE=true mbtileserver
```

In a docker-compose.yml file it will look like:

```
mbtileserver:
  ...

  environment:
    PORT: 7777
    TILE_DIR: "./path/to/your/tiles"
    VERBOSE: true
  entrypoint: mbtileserver

  ...
```

### Reload

mbtileserver optionally supports graceful reload (without interrupting any in-progress requests). This functionality
must be enabled with the `--enable-reload` flag. When enabled, the server can be reloaded by sending it a `HUP` signal:

```
$ kill -HUP <pid>
```

Reloading the server will cause it to pick up changes to the tiles directory, adding new tilesets and removing any that
are no longer present.

## Docker

Pull the latest image from [Docker Hub](https://hub.docker.com/r/consbio/mbtileserver):

```
docker pull consbio/mbtileserver:latest
```

To build the Docker image locally (named `mbtileserver`):

```
docker build -t mbtileserver -f Dockerfile .
```

To run the Docker container on port 8080 with your tilesets in `<host tile dir>`.
Note that by default, `mbtileserver` runs on port 8000 in the container.

```
docker run --rm -p 8080:8000 -v <host tile dir>:/tilesets  consbio/mbtileserver
```

You can pass in additional command-line arguments to `mbtileserver`, for example, to use
certificates and files in `<host cert dir>` so that you can access the server via HTTPS. The example below uses self-signed certificates generated using
[`mkcert`](https://github.com/FiloSottile/mkcert). This example uses automatic redirects, which causes `mbtileserver` to also listen on port 80 and automatically redirect to 443.

```
docker run  --rm -p 80:80 443:443 -v <host tile dir>:/tilesets -v <host cert dir>:/certs/ consbio/mbtileserver -c /certs/localhost.pem -k /certs/localhost-key.pem -p 443 --redirect
```

## Specifications

-   expects mbtiles files to follow version 1.0 of the [mbtiles specification](https://github.com/mapbox/mbtiles-spec). Version 1.1 is preferred.
-   implements [TileJSON 2.1.0](https://github.com/mapbox/tilejson-spec)

## Creating Tiles

You can create mbtiles files using a variety of tools. We have created
tiles for use with mbtileserver using:

-   [TileMill](https://www.mapbox.com/tilemill/) (image tiles)
-   [tippecanoe](https://github.com/mapbox/tippecanoe) (vector tiles)
-   [pymbtiles](https://github.com/consbio/pymbtiles) (tiles created using Python)
-   [tpkutils](https://github.com/consbio/tpkutils) (image tiles from ArcGIS tile packages)

## Examples

TileJSON API for each tileset:
`http://localhost/services/states_outline`

returns something like this;

```
{
  "bounds": [
    -179.23108,
    -14.601813,
    179.85968,
    71.441055
  ],
  "center": [
    0.314297,
    28.419622,
    1
  ],
  "credits": "US Census Bureau",
  "description": "States",
  "format": "png",
  "id": "states_outline",
  "legend": "[{\"elements\": [{\"label\": \"\", \"imageData\": \"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABQAAAAUCAYAAACNiR0NAAAAAXNSR0IB2cksfwAAAAlwSFlzAAAOxAAADsQBlSsOGwAAAGFJREFUOI3tlDEOgEAIBClI5kF+w0fxwXvQdjZywcZEtDI31YaQgWrdPsYzAPFGJCmmEAhJGzCash0wSVE/HHnlKcDMfrPXYgmXcAl/JswK6lCrz89BdGVm1+qrH0bbWDgA3WwmgzD8ueEAAAAASUVORK5CYII=\"}], \"name\": \"tl_2015_us_state\"}]",
  "map": "http://localhost/services/states_outline/map",
  "maxzoom": 4,
  "minzoom": 0,
  "name": "states_outline",
  "scheme": "xyz",
  "tags": "states",
  "tilejson": "2.1.0",
  "tiles": [
    "http://localhost/services/states_outline/tiles/{z}/{x}/{y}.png"
  ],
  "type": "overlay",
  "version": "1.0.0"
}
```

It provides most elements of the `metadata` table in the mbtiles file.

XYZ tile endpoint for individual tiles:
`http://localhost/services/states_outline/tiles/{z}/{x}/{y}.png`

If UTF-8 Grid data are present in the mbtiles file, they will be served up over the
grid endpoint:
`http://localhost/services/states_outline/tiles/{z}/{x}/{y}.json`

Grids are assumed to be gzip or zlib compressed in the mbtiles file. These grids
are automatically spliced with any grid key/value data if such exists in the mbtiles
file.

The map endpoint:
`http://localhost/services/states_outline/map`

provides an interactive Leaflet map for image tiles, including a few
helpful plugins like a legend (if compatible legend elements found in
TileJSON) and a transparency slider. Vector tiles are previewed using
Mapbox GL.

## ArcGIS API

This project currently provides a minimal ArcGIS tiled map service API for tiles stored in an mbtiles file.
This should be sufficient for use with online platforms such as [Data Basin](https://databasin.org). Because the ArcGIS API relies on a number of properties that are not commonly available within an mbtiles file, so certain aspects are stubbed out with minimal information.

This API is not intended for use with more full-featured ArcGIS applications such as ArcGIS Desktop.

## Live Examples

These are hosted on a free dyno by Heroku (thanks Heroku!), so there might be a small delay when you first access these.

-   [List of services](http://frozen-island-41032.herokuapp.com/services)
-   [TileJSON](http://frozen-island-41032.herokuapp.com/services/geography-class-png) for a PNG based tileset generated using TileMill.
-   [Map Preview ](http://frozen-island-41032.herokuapp.com/services/geography-class-png/map) for a map preview of the above.
-   [ArcGIS Map Service](http://frozen-island-41032.herokuapp.com/arcgis/rest/services/geography-class-png/MapServer)

## Request authorization

Provind a secret key with `-s/--secret-key` or by setting the `HMAC_SECRET_KEY` environment variable will
restrict access to all server endpoints and tile requests. Requests will only be served if they provide a cryptographic
signature created using the same secret key. This allows, for example, an application server to provide authorized
clients a short-lived token with which the clients can access tiles for a specific service.

Signatures expire 15 minutes from their creation date to prevent exposed or leaked signatures from being useful past a
small time window.

### Creating signatures

A signature is a URL-safe, base64 encoded HMAC hash using the `SHA1` algorithm. The hash key is an `SHA1` key created
from a randomly generated salt, and the **secret key** string. The hash payload is a combination of the ISO-formatted
date when the hash was created, and the authorized service id.

The following is an example signature, created in Go for the serivce id `test`, the date
`2019-03-08T19:31:12.213831+00:00`, the salt `0EvkK316T-sBLA`, and the secret key
`YMIVXikJWAiiR3q-JMz1v2Mfmx3gTXJVNqme5kyaqrY`

Create the SHA1 key:

```go
serviceId := "test"
date := "2019-03-08T19:31:12.213831+00:00"
salt := "0EvkK316T-sBLA"
secretKey := "YMIVXikJWAiiR3q-JMz1v2Mfmx3gTXJVNqme5kyaqrY"

key := sha1.New()
key.Write([]byte(salt + secretKey))
```

Create the signature hash:

```go
hash := hmac.New(sha1.New, key.Sum(nil))
message := fmt.Sprintf("%s:%s", date, serviceId)
hash.Write([]byte(message))
```

Finally, base64-encode the hash:

```go
b64hash := base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
fmt.Println(b64hash) // Should output: 2y8vHb9xK6RSxN8EXMeAEUiYtZk
```

### Making request

Authenticated requests must include the ISO-fromatted date, and a salt-signature combination in the form of:
`<salt>:<signature>`. These can be provided as query parameters:

```text
?date=2019-03-08T19:31:12.213831%2B00:00&signature=0EvkK316T-sBLA:YMIVXikJWAiiR3q-JMz1v2Mfmx3gTXJVNqme5kyaqrY
```

Or they can be provided as request headers:

```text
X-Signature-Date: 2019-03-08T19:31:12.213831+00:00
X-Signature: 0EvkK316T-sBLA:YMIVXikJWAiiR3q-JMz1v2Mfmx3gTXJVNqme5kyaqrY
```

## Roadmap

See the issues tagged to the [0.5 version](https://github.com/consbio/mbtileserver/milestone/1)
for our near term features and improvements.

In short, we are planning to:

-   add tests and benchmarks
-   get things production ready

## Development

Dependencies are managed using go modules. Vendored dependencies are stored in `vendor` folder by using `go mod vendor`.

On Windows, it is necessary to install `gcc` in order to compile `mattn/go-sqlite3`.  
MinGW or [TDM-GCC](https://sourceforge.net/projects/tdm-gcc/) should work fine.

If you experience very slow builds each time, it may be that you need to first run

```
go build -a .
```

to make subsequent builds much faster.

Development of the templates and static assets likely requires using
`node` and `npm`. Install these tools in the normal way.

From the `handlers/templates/static` folder, run

```
$npm install
```

to pull in the static dependencies. These are referenced in the
`package.json` file.

Then to build the minified version, run:

```
$gulp build
```

Modifying the `.go` files always requires re-running `go build .`.

In case you have modified the templates and static assets, you need to run `go generate ./handlers` to ensure that your modifications
are embedded into the executable. For this to work, you must have
[github.com/shurcooL/vfsgen)[https://github.com/shurcooL/vfsgen) installed.
This will rewrite the `assets_vfsdata.go` which you must commit along with your
modification. Also you should run `go build` after `go generate`.

During the development cycle you may use `go build -tags dev .` to build the
binary, in which case it will always take the assets from the relative file
path `handlers/templates/` directly and you can omit the `go generate` step. (note: this is currently not working properly)
But do not forget to perform it in the end.

## Changes

### 0.5.0 (in progress)

-   Added Docker support (#74, #75)
-   Fix case-sensitive mbtiles URLs (#77)
-   Add support for graceful reloading (#69, #72, #73)
-   Add support for environment args (#70)
-   All changes prior to 6/1/2019
