#!/bin/bash
set -Eeuo pipefail
set -x # print each command before exec

aws s3 sync s3://"${@: -1}" /tilesets

exec /mbtileserver --dir /tilesets "$@"