#!/bin/bash

update_file() {
    set +x # remove option temporarily
    diff $1 $2 >/dev/null 2>&1
    ret=$?
    if [ $ret -ne 0 ]; then
        echo "Updating $2"
        cp $1 $2
        set -x
        return 0
    else
        echo "Skip $2"
        set -x
        return 1
    fi
}

if [ "$EUID" -ne 0 ]; then # check sudo
    echo "Please run as root"
    exit 1
fi

set -eux

cd $(dirname $0)

(
    su isucon bash -c "cd ~/webapp/go; make"
)

rsync -av webapp/go/ ~/webapp/go/
rsync -av etc/nginx/ /etc/nginx/
#rsync -av etc/mysql/ /etc/mysql/
#rsync -av etc/sysctl.conf /etc/
#rsync -av etc/security/ /etc/security/

rm -rf /var/log/nginx/main_access.log
service nginx restart

systemctl restart cco.golang.service
# time curl localhost/initialize
