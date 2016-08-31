mbtileserver
============

Basic Go server for mbtiles.


## Objective ##
Provide a very simple Go-based server for map tiles stored in *.mbtiles files.

## Dependencies ##
* [github.com/labstack/echo](https://github.com/labstack/echo)
* [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3)
* [github.com/jmoiron/sqlx](https://github.com/jmoiron/sqlx)
* [github.com/spf13/cobra](https://github.com/spf13/cobra)
* [github.com/golang/groupcache](https://github.com/golang/groupcache)


On Windows, it is necessary to install `gcc`.  MinGW or [TDM-GCC](https://sourceforge.net/projects/tdm-gcc/) should work fine.


## Specifications ##
* expects mbtiles files to follow version 1.0 of the [mbtiles specification](https://github.com/mapbox/mbtiles-spec).  Version 1.1 is preferred.
* implements [TileJSON 2.1.0](https://github.com/mapbox/tilejson-spec)



## Work in progress ##
This project is very much a work in progress.  Stay tuned!

