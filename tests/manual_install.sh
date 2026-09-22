#!/bin/sh
# Install into a temp prefix and check the binary, the manual, and uninstall.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

prefix=$(mktemp -d)
trap 'make uninstall PREFIX="$prefix" DESTDIR= >/dev/null 2>&1 || true; rm -rf "$prefix"' EXIT

make install PREFIX="$prefix" DESTDIR=

test -x "$prefix/bin/agmemx"
test -f "$prefix/share/man/man1/agmemx.1"

man_page=$prefix/share/man/man1/agmemx.1
for name in \
	init \
	observe \
	"belief add" \
	"relation add" \
	search \
	"domain attach" \
	"embed reindex" \
	help \
	schema \
	--dir
do
	if ! grep -q -F -e "$name" "$man_page"; then
		printf '%s\n' "man page missing: $name" >&2
		exit 1
	fi
done

if [ -f completions/agmemx.bash ]; then
	test -f "$prefix/share/bash-completion/completions/agmemx"
else
	test ! -e "$prefix/share/bash-completion/completions/agmemx"
fi

make uninstall PREFIX="$prefix" DESTDIR=

for path in \
	"$prefix/bin/agmemx" \
	"$prefix/share/man/man1/agmemx.1" \
	"$prefix/share/bash-completion/completions/agmemx"
do
	if [ -e "$path" ]; then
		printf '%s\n' "still installed: $path" >&2
		exit 1
	fi
done
