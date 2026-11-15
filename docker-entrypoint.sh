#!/bin/sh
# Starts as root only long enough to give the mounted data directory to the
# memora user, then drops privileges for everything that serves requests. A
# bind mount keeps whatever ownership the host gave it, and every image before
# this one ran as root, so an install upgraded from ChronoFrame can arrive with
# files the server cannot open — it then exits on the first one, and Docker
# restarts it until somebody runs chown by hand.
#
# The ownership check is one readdir when the mount is already correct; the
# recursive chown runs only when an entry belongs to another user.
set -eu

data_dir="${MEMORA_DATA_DIR:-/app/data}"

# Started as a non-root user (--user, a Kubernetes securityContext, or a
# rootless daemon): there is nothing this script can change, and the mount must
# already be writable by that user.
if [ "$(id -u)" -ne 0 ]; then
	exec "$@"
fi

if ! mkdir -p "$data_dir"; then
	echo "entrypoint: cannot create $data_dir" >&2
fi

if [ -d "$data_dir" ] && [ -n "$(find "$data_dir" -maxdepth 1 ! -user memora 2>/dev/null | head -n 1)" ]; then
	echo "entrypoint: taking ownership of $data_dir for the memora user" >&2
	if ! chown -R memora:memora "$data_dir"; then
		echo "entrypoint: could not change ownership of $data_dir; the server may fail to write to it" >&2
	fi
fi

exec su-exec memora:memora "$@"
