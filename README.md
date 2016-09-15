mbtileserver
============

A simple Go-based server for map tiles stored in [mbtiles](https://github.com/mapbox/mbtiles-spec) 
format.

It currently provides support for `png`, `jpg`, and `pbf` (vector tile)
tilesets according to version 1.0 of the mbtiles specification.  Tiles
are served following the XYZ tile scheme, based on the Web Mercator
coordinate reference system.

At the moment, UTF8 Grids are not yet supported.

It provides a TileJSON 2.1.0 endpoint for each tileset, with full metadata
from the mbtiles file.

It also provides a preview map for exploring each tileset.

It uses `golang/groupcache` to provide caching of tiles for even faster
performance.

We have been able to host a bunch of tilesets on an 
[AWS t2.nano](https://aws.amazon.com/about-aws/whats-new/2015/12/introducing-t2-nano-the-smallest-lowest-cost-amazon-ec2-instance/)
virtual machine without any issues.


## Goals
* Provide a web tile API for map tiles stored in mbtiles format
* Be fast
* Run on small resource cloud hosted machines (limited memory & CPU)
* Be easy to install and operate


## Installation
Currently, this project is not `go get`-able because static assets and 
templates are not downloaded via `go get`.  We're working toward this.

In the meantime, clone this repository using your `git` tool of choice.

`go get` the dependencies:
* [github.com/labstack/echo](https://github.com/labstack/echo)
* [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3)
* [github.com/jmoiron/sqlx](https://github.com/jmoiron/sqlx)
* [github.com/spf13/cobra](https://github.com/spf13/cobra)
* [github.com/golang/groupcache](https://github.com/golang/groupcache)

On Windows, it is necessary to install `gcc` in order to compile `mattn/go-sqlite3`.  
MinGW or [TDM-GCC](https://sourceforge.net/projects/tdm-gcc/) should work fine.


## Usage
From within the repository root:
```
$ ./mbtileserver --help
Serve tiles from mbtiles files

Usage:
  mbtileserver [flags]

Flags:
      --cachesize int   Size of cache in MB. (default 250)
  -c, --cert string     X.509 TLS certificate filename.  If present, will be used to enable SSL on the server.
  -d, --dir string      Directory containing mbtiles files. (default "./tilesets")
  -k, --key string      flag usage
  -p, --port int        Server port. (default 8000)
```

So hosting tiles is as easy as putting your mbtiles files in the `tilesets`
directory and starting the server.  Woo hoo!

When you want to remove, modify, or add new tilesets, simply restart the server process.


## Specifications
* expects mbtiles files to follow version 1.0 of the [mbtiles specification](https://github.com/mapbox/mbtiles-spec).  Version 1.1 is preferred.
* implements [TileJSON 2.1.0](https://github.com/mapbox/tilejson-spec)


## Creating Tiles
You can create mbtiles files using a variety of tools.  We have created
tiles for use with mbtileserver using:
* [TileMill](https://www.mapbox.com/tilemill/)  (image tiles)
* [tippecanoe](https://github.com/mapbox/tippecanoe)   (vector tiles)
* [tpkutils](https://github.com/consbio/tpkutils)  (image tiles from ArcGIS tile packages)

We are working on some new tools to create UTF8 Grids for use here.


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

It provides all elements of the `metadata` table in the mbtiles file.


XYZ tile endpoint for individual tiles:
`http://localhost/services/states_outline/tiles/{z}/{x}/{y}.png`


The map endpoint:
`http://localhost/services/states_outline/map`

provides an interactive Leaflet map for image tiles, including a few
helpful plugins like a legend (if compatible legend elements found in
TileJSON) and a transparency slider.  Vector tiles are previewed using
Mapbox GL.


## Roadmap
See the issues tagged to the [0.5 version](https://github.com/consbio/mbtileserver/milestone/1)
for our near term features and improvements.

In short, we are planning to:
* make this project `go get`-able
* add an ArcGIS API for tilesets
* refactor the internals to better separate mbtiles interface from API logic
* add tests and benchmarks
* get things production ready


## Development
Development of the templates and static assets likely requires using
`node` and `npm`.  Install these tools in the normal way.

From the `templates/static` folder, run
```
$npm install
```

to pull in the static dependencies.  These are referenced in the
`package.json` file.

Then to build the minified version, run:
```
$gulp build
```


Modifying the `go` files requires re-running `go build`.


