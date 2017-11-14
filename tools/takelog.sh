#!/bin/bash
cd $(dirname $0)

# Do not add '/'
ruby stat.rb  < /var/log/nginx/main_access.log | tee stat.txt

# /usr/local/go/bin/go tool pprof ~/isubata/webapp/go/isubata /tmp/cpu.prof <<EOF
# list main.* > prof.txt
# EOF
