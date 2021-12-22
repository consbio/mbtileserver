# Changelog

## 0.8

### General changes

-   display attribution in preview maps if present in tileset metadata.
-   upgraded Docker containers to Go 1.17.
-   upgraded Go version used for release to Go 1.17.
-   switched to go:embed for embedding templates and static assets.
-   dropped internal mbtiles package in favor of github.com/brendan-ward/mbtiles-go,
    which wraps the SQlite-specific go package `crawshaw.io/sqlite` for better
    performance.

### Command-line interface

-   added support for watching filesystem for changes to tilesets using
    `--enable-fs-watch` option.

### Breaking changes

-   now requires Go 1.16+.
-   removes ArcGIS API layer info at the service root and layers endpoint (#116);
    this was not providing useful information for image tilesets.
-   removed `handlers.Assets`; static assets are intended only for use in template
    or static file handlers.
-   removed support for UTF Grids.

### Bug Fixes

-   fix handlers for ArcGIS API endpoints, resolving tile shift issue (#116).
-   obviated incorrect include of node_modules in compiled asset file; executable
    is now smaller and faster to build.

## O.7

### General changes

-   substantial changes to internal functionality and HTTP handlers, see details below
-   now requires Go 1.13+
-   upgraded Docker containers to Go 1.16
-   upgraded Go version used for release to Go 1.16
-   removed `vendor` directory; no longer needed for Go 1.13
-   switched from Travis-CI to Github actions for running tests

### Command-line interface

-   added support for automatically generating unique tileset IDs using `--generate-ids` option
-   added ability to toggle off non-tile endpoints:

    -   `--disable-preview`: disables the map preview, enabled by default.
    -   `--disable-svc-list`: disables the list of map services, enabled by default
    -   `--disable-tilejson`: disables the TileJSON endpoint for each tile service
    -   `--tiles-only`: shortcut that disables preview, service list, and TileJSON endpoints

-   added ability to have multiple tile paths using a comma-delimited list of paths passed to `--dir` option

-   moved static assets for map preview that were originally served on `/static`
    endpoint to `/services/<tileset_id>/map/static` so that this endpoint is
    disabled when preview is disabled via `--disable-preview`.

### Go API

-   added `ServiceSetConfig` for configuration options for `ServiceSet` instances
-   added `ServiceSet.AddTileset()`, `ServiceSet.UpdateTileset()`,
    `ServiceSet.RemoveTileset()`, and `ServiceSet.HasTileset()` functions.
    WARNING: these functions are not yet thread-safe.

### Breaking changes

#### Command-line interface:

-   ArcGIS endpoints are now opt-in via `--enable-arcgis` option (disabled by default)
-   `--path` option has been renamed to `--root-url` for clarity (env var is now `ROOT_URL`)
-   `--enable-reload` has been renamed to `--enable-reload-signal`

#### Handlers API

-   `ServiceSet.Handler` parameters have been replaced with `ServiceSetConfig`
    passed to `handlers.New()` instead.
-   removed `handlers.NewFromBaseDir()`, replaced with `handlers.New()` and calling
    `ServiceSet.AddTileset()` for each `Tileset` to register.
-   removed `ServiceSet.AddDBOnPath()`; this is replaced by calling
    `ServiceSet.AddTileset()` for each `Tileset` to register.

### Bug fixes

-   Fixed WebP parsing, now uses simplified check for a `RIFF` header (WebP is only likely RIFF format to be stored in tiles). #98, #110

### Details

This version involved a significant refactor of internal functionality and HTTP
handlers to provide better ability to modify services at runtime, provide
granular control over the endpoints that are exposed, and cleanup handling
of middleware.

Most internal HTTP handlers for `ServiceSet` and `Tileset` in the
`github.com/consbio/mbtileserver/handlers` package are now `http.HandlerFunc`s
instead of custom handlers that returned status codes or errors as in the previous
versions.

The internal routing within these handlers has been modified to enable
tilesets to change at runtime. Previously, we were using an `http.ServeMux`
for all routes, which breaks when the `Tileset` instances pointed to by those
routes have changed at runtime. Now, the top-level `ServiceSet.Handler()`
allows dynamic routing to any `Tileset` instances currently published. Each
`Tileset` is now responsible for routing to its subpaths (e.g., tile endpoint).

The singular public handler endpoint is still an `http.Handler` instance but
no longer takes any parameters. Those parameters are now handled using
configuration options instead.

`ServiceSet` now enables configuration to set the root URL, toggle which endpoints
are exposed and set the internal error logger. These are passed in using a
`ServiceSetConfig` struct when the service is constructed; these configuration
options are not modifiable at runtime.

`Tileset` instances are now created individually from a set of source `mbtiles`
files, instead of generated within `ServiceSet` from a directory. This provides
more granular control over assigning IDs to tilesets as well as creating,
updating, or deleting `Tileset` instances. You must generate unique IDs for
tilesets before adding to the `ServiceSet`; you can use
`handlers.SHA1ID(filename)` to generate a unique SHA1 ID of the service based on
its full filename path, or `handlers.RelativePathID(filename, tilePath)` to
generate the ID from its path and filename within the tile directory `tilePath`.

HMAC authorization has been refactored into middleware external to the Go API.
It now is instantiated as middleware in `main.go`; this provides better
separation of concerns between the server (`main.go`) and the Go API. The API
for interacting with HMAC authorization from the CLI or endpoints remains the
same.

Most of the updates are demonstrated in `main.go`.

## 0.6.1

-   upgraded Docker containers to Go 1.14 (solves out of memory issues during builds on small containers)

## 0.6

-   fixed bug in map preview when bounds are not defined for a tileset (#84)
-   updated Leaflet to 1.6.0 and Mapbox GL to 0.32.0 (larger upgrades contingent on #65)
-   fixed issues with `--tls` option (#89)
-   added example proxy configuration for Caddy and NGINX (#91)
-   fixed issues with map preview page using HTTP basemaps (#90)
-   resolved template loading issues (#85)

### Breaking changes - handlers API:

-   Removed `TemplatesFromAssets` as it was not used internally, and unlikely used externally
-   Removed `secretKey` from `NewFromBaseDir` parameters; this is replaced by calling `SetRequestAuthKey` on a `ServiceSet`.

## 0.5.0

-   Added Docker support (#74, #75)
-   Fix case-sensitive mbtiles URLs (#77)
-   Add support for graceful reloading (#69, #72, #73)
-   Add support for environment args (#70)
-   All changes prior to 6/1/2019
