mbtileserver
============

Basic Go server for mbtiles.


## Objective ##
Provide a very simple Go-based server for map tiles stored in *.mbtiles files.

## Dependencies ##
* [github.com/jessevdk/go-flags](https://github.com/jessevdk/go-flags)
* [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3)
* [github.com/jmoiron/sqlx](https://github.com/jmoiron/sqlx)
* [github.com/zenazn/goji](https://github.com/zenazn/goji)
* [github.com/golang/groupcache](https://github.com/golang/groupcache)
* [github.com/rs/cors](https://github.com/rs/cors)


On Windows, it is necessary to install `gcc`.  MinGW or [TDM-GCC](https://sourceforge.net/projects/tdm-gcc/) should work fine.


## Work in progress ##
This project is very much a work in progress.  Stay tuned!

## Notes ##
Currently using groupcache for in-memory caching and coordination of database reads.  May replace with goji caching middleware to keep this stack more self-contained.  TBD.