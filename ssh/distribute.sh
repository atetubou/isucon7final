#!/bin/bash
# Usage:
# $ ./distribute.sh <password> [user@hostname ...]

set -eu

cd $(dirname $0)


echo "$@"

password=$1

hosts=("$@")
unset hosts[0]

for hostname in "${hosts[@]}"
do
    for pub in *.pub
    do
	# echo $pub
	sshpass -p $1 ssh-copy-id -f -i $pub -o "StrictHostKeyChecking no" $hostname
    done
done
