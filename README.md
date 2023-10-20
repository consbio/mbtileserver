# mbtileserver

A simple Go-based server for map tiles stored in [mbtiles](https://github.com/mapbox/mbtiles-spec)
format.

![Build Status](https://github.com/consbio/mbtileserver/actions/workflows/test.yml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/consbio/mbtileserver/badge.svg?branch=master)](https://coveralls.io/github/consbio/mbtileserver?branch=master)
[![GoDoc](https://godoc.org/github.com/consbio/mbtileserver?status.svg)](http://godoc.org/github.com/consbio/mbtileserver)
[![Go Report Card](https://goreportcard.com/badge/github.com/consbio/mbtileserver)](https://goreportcard.com/report/github.com/consbio/mbtileserver)

It currently provides support for `png`, `jpg`, `webp`, and `pbf` (vector tile)
tilesets according to version 1.0 of the mbtiles specification. Tiles
are served following the XYZ tile scheme, based on the Web Mercator
coordinate reference system. UTF8 Grids are no longer supported.

In addition to tile-level access, it provides:

-   TileJSON 2.1.0 endpoint for each tileset, with full metadata
    from the mbtiles file.
-   a preview map for exploring each tileset.
-   a minimal ArcGIS tile map service API

We have been able to host a bunch of tilesets on an
[AWS t2.nano](https://aws.amazon.com/about-aws/whats-new/2015/12/introducing-t2-nano-the-smallest-lowest-cost-amazon-ec2-instance/)
virtual machine without any issues.

## Goals

-   Provide a web tile API for map tiles stored in mbtiles format
-   Be fast
-   Run on small resource cloud hosted machines (limited memory & CPU)
-   Be easy to install and operate

## Supported Go versions

_Requires Go >= 1.18+._

`mbtileserver` uses go modules and follows standard practices as of Go 1.18.

## Installation

You can install this project with

```sh
go install github.com/consbio/mbtileserver@latest
```

This will create and install an executable called `mbtileserver`.

## Usage

From within the repository root ($GOPATH/bin needs to be in your $PATH):

```
$  mbtileserver --help
Serve tiles from mbtiles files.

Usage:
  mbtileserver [flags]

Flags:
  -c, --cert string            X.509 TLS certificate filename.  If present, will be used to enable SSL on the server.
  -d, --dir string             Directory containing mbtiles files. Directory containing mbtiles files.  Can be a comma-delimited list of directories. (default "./tilesets")
      --disable-preview        Disable map preview for each tileset (enabled by default)
      --disable-svc-list       Disable services list endpoint (enabled by default)
      --disable-tilejson       Disable TileJSON endpoint for each tileset (enabled by default)
      --domain string          Domain name of this server.  NOTE: only used for AutoTLS.
      --dsn string             Sentry DSN
      --enable-arcgis          Enable ArcGIS Mapserver endpoints
      --enable-fs-watch        Enable reloading of tilesets by watching filesystem
      --enable-reload-signal   Enable graceful reload using HUP signal to the server process
      --generate-ids           Automatically generate tileset IDs instead of using relative path
  -h, --help                   help for mbtileserver
  -k, --key string             TLS private key
  -p, --port int               Server port. Default is 443 if --cert or --tls options are used, otherwise 8000. (default -1)
  -r, --redirect               Redirect HTTP to HTTPS
      --root-url string        Root URL of services endpoint (default "/services")
  -s, --secret-key string      Shared secret key used for HMAC request authentication
      --tiles-only             Only enable tile endpoints (shortcut for --disable-svc-list --disable-tilejson --disable-preview)
  -t, --tls                    Auto TLS via Let's Encrypt
  -v, --verbose                Verbose logging
```

So hosting tiles is as easy as putting your mbtiles files in the `tilesets`
directory and starting the server. Woo hoo!

You can have multiple directories in your `tilesets` directory; these will be converted into appropriate URLs:

`<tile_dir>/foo/bar/baz.mbtiles` will be available at `/services/foo/bar/baz`.

If `--generate-ids` is provided, tileset IDs are automatically generated using a SHA1 hash of the path to each tileset.
By default, tileset IDs are based on the relative path of each tileset to the base directory provided using `--dir`.

When you want to remove, modify, or add new tilesets, simply restart the server process or use one of the reloading processes below.

If a valid Sentry DSN is provided, warnings, errors, fatal errors, and panics will be reported to Sentry.

If `redirect` option is provided, the server also listens on port 80 and redirects to port 443.

If the `--tls` option is provided, the Let's Encrypt Terms of Service are accepted automatically on your behalf. Please review them [here](https://letsencrypt.org/repository/). Certificates are cached in a `.certs` folder created where you are executing `mbtileserver`. Please make sure this folder can be written by the `mbtileserver` process or you will get errors. Certificates are not requested until the first request is made to the server. We recommend that you initialize these after startup by making a request against `https://<hostname>/services` and watching the logs from the server to make sure that certificates were processed correctly. Common errors include Let's Encrypt not being able to access your server at the domain you provided. `localhost` or internal-only domains will not work.

If either `--cert` or `--tls` are provided, the default port is 443.

You can also use environment variables instead of flags, which may be more helpful when deploying in a docker image. Use the associated flag to determine usage. The following variables are available:

-   `PORT` (`--port`)
-   `TILE_DIR` (`--dir`)
-   `GENERATE_IDS` (`--generate-ids`)
-   `ROOT_URL` (`--root-url`)
-   `DOMAIN` (`--domain`)
-   `TLS_CERT` (`--cert`)
-   `TLS_PRIVATE_KEY` (`--key`)
-   `HMAC_SECRET_KEY` (`--secret-key`)
-   `AUTO_TLS` (`--tls`)
-   `REDIRECT` (`--redirect`)
-   `DSN` (`--dsn`)
-   `VERBOSE` (`--verbose`)

Example:

```
PORT=7777 TILE_DIR=./path/to/your/tiles VERBOSE=true mbtileserver
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

### Reloading

#### Reload using a signal

mbtileserver optionally supports graceful reload (without interrupting any in-progress requests). This functionality
must be enabled with the `--enable-reload-signal` flag. When enabled, the server can be reloaded by sending it a `HUP` signal:

```
kill -HUP <pid>
```

Reloading the server will cause it to pick up changes to the tiles directory, adding new tilesets and removing any that
are no longer present.

#### Reload using a filesystem watcher

mbtileserver optionally supports reload of individual tilesets by watching for filesystem changes. This functionality
must be enabled with the `--enable-fs-watch` flag.

All directories specified by `-d` / `--dir` and any subdirectories that exist at the time the server is started
will be watched for changes to the tilesets.

An existing tileset that is being updated will be locked while the file on disk
is being updated. This will cause incoming requests to that tileset to stall
for up to 30 seconds and will return as soon as the tileset is completely updated
and unlocked. If it takes longer than 30 seconds for the tileset to be updated,
HTTP 503 errors will be returned for that tileset until the tileset is completely
updated and unlocked.

Under very high request volumes, requests that come in between when the file is
first modified and when that modification is first detected (and tileset locked)
may encounter errors.

WARNING: Do not remove the top-level watched directories while the server is running.

WARNING: Do not create or delete subdirectories within the watched directories while the server is running.

WARNING: do not generate tiles directly in the watched directories. Instead, create them in separate directories and
copy them into the watched directories when complete.

### Using with a reverse proxy

You can use a reverse proxy in front of `mbtileserver` to intercept incoming requests, provide TLS, etc.

We have used both [`Caddy`](https://caddyserver.com/) and [`NGINX`](https://www.nginx.com/) for our production setups in various projects,
usually when we need to proxy to additional backend services.

To make sure that the correct request URL is passed to `mbtileserver` so that TileJSON and map preview endpoints work correctly,
make sure to have your reverse proxy send the following headers:

Scheme (HTTP vs HTTPS):
one of `X-Forwarded-Proto`, `X-Forwarded-Protocol`, `X-Url-Scheme` to set the scheme of the request.
OR
`X-Forwarded-Ssl` to automatically set the scheme to HTTPS.

Host:
Set `X-Forwarded-Host` to the correct host for the request.

#### Caddy v2 Example:

For `mbtileserver` running on port 8000 locally, add the following to the block for your domain name:

```
<domain_name> {
  route /services* {
    reverse_proxy localhost:8000
  }
}
```

You may want to consider adding cache control headers within the `route` block
depending on how often the contents of your tilesets change. For instance,
to prevent clients from caching tiles longer than 1 hour:

```
  route /services* {
    header Cache-Control "public, max-age=3600, must-revalidate"
    localhost mbtileserver:8000
  }
```

#### NGINX Example:

For `mbtileserver` running on port 8000 locally, add the following to your `server` block:

```
server {
   <other config options>

    location /services {
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $server_name;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_pass http://localhost:8000;
    }
}
```

## Docker

Pull the latest image from
[Github Container Registry](https://github.com/consbio/mbtileserver/pkgs/container/mbtileserver):

```
docker pull ghcr.io/consbio/mbtileserver:latest
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

Alternately, use `docker-compose` to run:

```
docker-compose up -d
```

The default `docker-compose.yml` configures `mbtileserver` to connect to port 8080 on the host, and uses the `./mbtiles/testdata` folder for tilesets. You can use your own `docker-compose.override.yml` or [environment specific files](https://docs.docker.com/compose/extends/) to set these how you like.

To reload the server:

```
docker exec -it mbtileserver sh -c "kill -HUP 1"
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

The root name of each mbtiles file becomes the "tileset_id" as used below.

## XYZ Tile API

The primary use of `mbtileserver` is as a host for XYZ tiles.

These are provided at:
`/services/<tileset_id>/tiles/{z}/{x}/{y}.<format>`

where `<format>` is one of `png`, `jpg`, `webp`, `pbf` depending on the type of data in the tileset.

## TileJSON API

`mbtileserver` automatically creates a TileJSON endpoint for each service at `/services/<tileset_id>`.
The TileJSON uses the same scheme and domain name as is used for the incoming request; the `--domain` setting does not
affect auto-generated URLs.

This API provides most elements of the `metadata` table in the mbtiles file as well as others that are
automatically inferred from tile data.

For example,
`http://localhost/services/states_outline`

returns something like this:

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

## Map preview

`mbtileserver` automatically creates a map preview page for each tileset at `/services/<tileset_id>/map`.

It uses `MapLibre GL` to render vector and image tiles.

No built-in basemap is included by default in the map preview.  You can use
one of the following options to include a basemap.

### Basemap style URL

To include a [MapLibre GL style URL](https://maplibre.org/maplibre-style-spec/)
use the `--basemap-style-url` option to provide a URL to that style:

```
--basemap-style-url "https://tiles.stadiamaps.com/styles/stamen_toner_lite.json?api_key=<your key>
```

The URL can include query parameters as required by the host, such as
`?access_token=<something>`.


### Basemap tiles URL

To include a basemap based on image tile URLs, use the `--basemap-tiles-url`
option to provide a raster tile URL pattern:

```
--basemap https://some.host/{z}/{x}/{y}.png
```

The template parameters `{z}` (zoom), `{x}`, `{y}` are required.

The extension can be omitted or be any image format supported by MapLibre GL.

The URL can include query parameters as required by the host, such as
`?access_token=<something>`.

IMPORTANT: this does not support vector tiles.


## ArcGIS API

This project currently provides a minimal ArcGIS tiled map service API for tiles stored in an mbtiles file.

This is enabled with the `--enable-arcgis` flag.

This should be sufficient for use with online platforms such as [Data Basin](https://databasin.org). Because the ArcGIS API relies on a number of properties that are not commonly available within an mbtiles file, so certain aspects are stubbed out with minimal information.

This API is not intended for use with more full-featured ArcGIS applications such as ArcGIS Desktop.

Available endpoints:

-   Service info: `http://localhost:8000/arcgis/rest/services/<tileset_id>/MapServer`
-   Layer info: `http://localhost:8000/arcgis/rest/services/<tileset_id>/MapServer/layers`
-   Tiles: `http://localhost:8000/arcgis/rest/services/<tileset_id>/MapServer/tile/0/0/0`

## Request authorization

Providing a secret key with `-s/--secret-key` or by setting the `HMAC_SECRET_KEY` environment variable will
restrict access to all server endpoints and tile requests. Requests will only be served if they provide a cryptographic
signature created using the same secret key. This allows, for example, an application server to provide authorized
clients a short-lived token with which the clients can access tiles for a specific service.

Signatures expire 15 minutes from their creation date to prevent exposed or leaked signatures from being useful past a
small time window.

### Creating signatures

A signature is a URL-safe, base64 encoded HMAC hash using the `SHA1` algorithm. The hash key is an `SHA1` key created
from a randomly generated salt, and the **secret key** string. The hash payload is a combination of the ISO-formatted
date when the hash was created, and the authorized service id.

The following is an example signature, created in Go for the service id `test`, the date
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

Authenticated requests must include the ISO-formatted date, and a salt-signature combination in the form of:
`<salt>:<signature>`. These can be provided as query parameters:

```text
?date=2019-03-08T19:31:12.213831%2B00:00&signature=0EvkK316T-sBLA:YMIVXikJWAiiR3q-JMz1v2Mfmx3gTXJVNqme5kyaqrY
```

Or they can be provided as request headers:

```text
X-Signature-Date: 2019-03-08T19:31:12.213831+00:00
X-Signature: 0EvkK316T-sBLA:YMIVXikJWAiiR3q-JMz1v2Mfmx3gTXJVNqme5kyaqrY
```

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

```bash
npm install
```

to pull in the static dependencies. These are referenced in the
`package.json` file.

Then to build the minified version, run:

```bash
npm run build
```

Built static assets are saved to `handlers/templates/static/dist` and included
via `go:embed` into the final executable.

Modifying the `.go` files or anything under `handlers/templates` always requires
re-running `go build .`.

## Changes

See [CHANGELOG](CHANGELOG.md).

## Contributors ✨

Thanks goes to these wonderful people ([emoji key](https://allcontributors.org/docs/en/emoji-key)):

<!-- ALL-CONTRIBUTORS-LIST:START - Do not remove or modify this section -->
<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
<table>
  <tr>
    <td align="center"><a href="https://astutespruce.com"><img src="https://avatars2.githubusercontent.com/u/3375604?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Brendan Ward</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=brendan-ward" title="Code">💻</a> <a href="https://github.com/consbio/mbtileserver/commits?author=brendan-ward" title="Documentation">📖</a> <a href="https://github.com/consbio/mbtileserver/issues?q=author%3Abrendan-ward" title="Bug reports">🐛</a> <a href="#blog-brendan-ward" title="Blogposts">📝</a> <a href="https://github.com/consbio/mbtileserver/pulls?q=is%3Apr+reviewed-by%3Abrendan-ward" title="Reviewed Pull Requests">👀</a> <a href="#ideas-brendan-ward" title="Ideas, Planning, & Feedback">🤔</a></td>
    <td align="center"><a href="https://github.com/fawick"><img src="https://avatars3.githubusercontent.com/u/1886500?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Fabian Wickborn</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=fawick" title="Code">💻</a> <a href="https://github.com/consbio/mbtileserver/commits?author=fawick" title="Documentation">📖</a> <a href="https://github.com/consbio/mbtileserver/issues?q=author%3Afawick" title="Bug reports">🐛</a> <a href="#ideas-fawick" title="Ideas, Planning, & Feedback">🤔</a></td>
    <td align="center"><a href="https://github.com/nikmolnar"><img src="https://avatars1.githubusercontent.com/u/2422416?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Nik Molnar</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=nikmolnar" title="Code">💻</a> <a href="#ideas-nikmolnar" title="Ideas, Planning, & Feedback">🤔</a> <a href="https://github.com/consbio/mbtileserver/issues?q=author%3Anikmolnar" title="Bug reports">🐛</a></td>
    <td align="center"><a href="https://sikmir.ru"><img src="https://avatars3.githubusercontent.com/u/688044?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Nikolay Korotkiy</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=sikmir" title="Code">💻</a> <a href="https://github.com/consbio/mbtileserver/issues?q=author%3Asikmir" title="Bug reports">🐛</a></td>
    <td align="center"><a href="https://github.com/retbrown"><img src="https://avatars1.githubusercontent.com/u/3111954?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Robert Brown</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=retbrown" title="Code">💻</a></td>
    <td align="center"><a href="mika-go-13"><img src="https://avatars.githubusercontent.com/u/26978815?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Mihail</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=mika-go-13" title="Code">💻</a></td>
    <td align="center"><a href="https://github.com/buma"><img src="https://avatars2.githubusercontent.com/u/1055967?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Marko Burjek</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=buma" title="Code">💻</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/Krizz"><img src="https://avatars0.githubusercontent.com/u/689050?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Kristjan</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=Krizz" title="Code">💻</a></td>
    <td align="center"><a href="https://github.com/evbarnett"><img src="https://avatars2.githubusercontent.com/u/4960874?v=4?s=100" width="100px;" alt=""/><br /><sub><b>evbarnett</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/issues?q=author%3Aevbarnett" title="Bug reports">🐛</a></td>
    <td align="center"><a href="https://www.walkaholic.me"><img src="https://avatars1.githubusercontent.com/u/19690868?v=4?s=100" width="100px;" alt=""/><br /><sub><b>walkaholic.me</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/issues?q=author%3Acarlos-mg89" title="Bug reports">🐛</a></td>
    <td align="center"><a href="http://www.webiswhatido.com"><img src="https://avatars1.githubusercontent.com/u/1580910?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Brian Voelker</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/issues?q=author%3Abrianvoe" title="Bug reports">🐛</a></td>
    <td align="center"><a href="http://salesking.eu"><img src="https://avatars1.githubusercontent.com/u/13575?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Georg Leciejewski</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/issues?q=author%3Aschorsch" title="Bug reports">🐛</a></td>
    <td align="center"><a href="https://github.com/cbenz"><img src="https://avatars.githubusercontent.com/u/12686?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Christophe Benz</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/issues?q=author%3Acbenz" title="Bug reports">🐛</a></td>
    <td align="center"><a href="https://github.com/reyemtm"><img src="https://avatars.githubusercontent.com/u/6398929?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Malcolm Meyer</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/issues?q=author%3Areyemtm" title="Bug reports">🐛</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/jleedev"><img src="https://avatars.githubusercontent.com/u/23022?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Josh Lee</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=jleedev" title="Code">💻</a></td>
    <td align="center"><a href="https://github.com/Martin8412"><img src="https://avatars.githubusercontent.com/u/2369612?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Martin Karlsen Jensen</b></sub></a><br /><a href="https://github.com/consbio/mbtileserver/commits?author=Martin8412" title="Code">💻</a></td>
  </tr>
</table>

<!-- markdownlint-restore -->
<!-- prettier-ignore-end -->

<!-- ALL-CONTRIBUTORS-LIST:END -->

This project follows the [all-contributors](https://github.com/all-contributors/all-contributors) specification. Contributions of any kind welcome!
