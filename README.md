mbtileserver
============

Basic Go server for mbtiles.


## Objective ##
Provide a very simple Go-based server for map tiles stored in *.mbtiles files.

## Dependencies ##
* [github.com/jessevdk/go-flags](http://github.com/jessevdk/go-flags)
* [github.com/mattn/go-sqlite3](http://github.com/mattn/go-sqlite3)
* [github.com/zenazn/goji](http://github.com/zenazn/goji)
* [github.com/golang/groupcache](http://github.com/golang/groupcache)

## Work in progress ##
This project is very much a work in progress.  Stay tuned!

## Notes ##
Currently using groupcache for in-memory caching and coordination of database reads.  May replace with goji caching middleware to keep this stack more self-contained.  TBD.