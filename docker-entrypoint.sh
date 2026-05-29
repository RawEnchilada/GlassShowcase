#!/bin/sh
set -eu

mkdir -p /data /app/data
chown -R glass:glass /data /app/data

exec gosu glass "$@"
