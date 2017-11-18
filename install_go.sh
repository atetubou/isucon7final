#!/bin/bash

# install golang 1.9.2 to /usr/bin
# if there is /usr/bin/go, it is moved to /usr/bin/go_old

make_symlink() {
	TARGET_PATH=$1
	for bin in $(find /usr/local/go/bin -type f)
	do
		TARGET="$TARGET_PATH/$(basename "$bin")"
		[ -f $TARGET ] && sudo mv $TARGET "$TARGET"_old
		sudo cp --symbolic-link $bin $TARGET_PATH
	done
}

install_go() {
	TARGET_PATH=/usr/bin
	(
		cd /tmp
		git clone https://github.com/udhos/update-golang
		cd update-golang
		sudo ./update-golang.sh
		make_symlink $TARGET_PATH
	)
}



set -eux
install_go

