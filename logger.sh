#!/usr/bin/sudo /bin/bash
set -eu
cd $(dirname $0)
source ./util.sh
source /home/isucon/.profile

NGINX_LOG_FILE=/var/log/nginx/main_access.log
MYSQL_LOG_FILE=/var/log/mysql/mysql-slow.log
LOG_DIR=/home/isucon/log/log
CPUPROF_FILE=/tmp/cpu.prof
ALP_CONF=./alpconfig.yml

show_help_and_exit() {
    echo "Usage: $0 start [ID]  start logging using working directory ID.  If ID is not provided, use nextid" >&2
    echo "       $0 stop  [ID]  stop logging and record logs" >&2
    echo "       $0 term  ID    terminate logging and remove directory ID" >&2
    echo "       $0 nextid      print next candidate id" >&2
    echo "       $0 lastid      print last bench id" >&2
    exit ${1:-0}
}

clear_mysql_logfile() {
    truncate -s 0 $MYSQL_LOG_FILE
    chown mysql:mysql $MYSQL_LOG_FILE
    # postrotate script from /etc/logrotate.d/mysql-server
    test -x /usr/bin/mysqladmin || exit 0
    # If this fails, check debian.conf!
    MYADMIN="/usr/bin/mysqladmin --defaults-file=/etc/mysql/debian.cnf"
    if [ -z "`$MYADMIN ping 2>/dev/null`" ]; then
    	# Really no mysqld or rather a missing debian-sys-maint user?
    	# If this occurs and is not a error please report a bug.
    	#if ps cax | grep -q mysqld; then
    	if killall -q -s0 -umysql mysqld; then
    	    exit 1
    	fi
    else
    	$MYADMIN flush-logs
    fi
    return $?
}

clear_nginx_logfile() {
    truncate -s 0 $NGINX_LOG_FILE
    # postrotate script from /etc/logrotate.d/nginx
    service nginx rotate
    return $?
}

log_file_name_of() {
    # log_file_name_of NAME [EXTENSION]
    ext="${2:-log}"
    echo $LOG_DIR/$HOSTNAME.$1.$ext
}

start_dstat() {
    logfile=$(log_file_name_of dstat)
    pidfile="$logfile.pid"
    if [ -f $pidfile ]
    then
	stop_dstat || true
    fi
    rm -f "$logfile"
    DSTAT_MYSQL_USER=root DSTAT_MYSQL_PWD= script -q -c "stty cols 1000; TERM=xterm-256color dstat -t -c --top-cpu -d -n -m -s --top-mem --mysql5-cmds --mysql5-io --mysql5-keys" "$logfile" >/dev/null 2>&1 &
    echo "$!" > $pidfile
}
stop_dstat() {
    logfile=$(log_file_name_of dstat)
    pidfile="$logfile.pid"
    if [ ! -f $pidfile ]
    then
	return 1
    fi
    pid=$(cat $pidfile)
    rm -f $pidfile
    kill -INT $pid
    if [ $? -ne 0 ]
    then
	return 1
    fi
    # add shebang
    retried=0
    while kill -0 $pid >/dev/null 2>&1
    do
	echo "Waiting for stopping process $pid..."
	sleep 1
	retried=$((retried+1))
	if [ $retried -ge 10 ]
	then
	    kill -TERM $pid
	    break
	fi
    done
    cat <(echo '#!/usr/bin/less -SR') "$logfile" | dd conv=notrunc of="$logfile" 2>/dev/null # add shebang to view logfile
    chmod 755 "$logfile"
    return $?
}

maximum_of() {
    res=${1:-0}
    shift
    for i in $@
    do
	if [ $res -lt $i ]
	then
	    res=$i
	fi
    done
    res=$(expr $res + 0) # cast as integer
    echo $res
}
get_last_dir() {
    # find the last directory (i.e. with a name of the largest number)
    maximum_of $(find $LOG_DIR -maxdepth 1 -type d | sed -n -e  's@^.*/\([0-9][0-9]*\)@\1@p')
}
next_id() {
    last_id=$(get_last_dir)
    last_id=$((last_id + 1))
    printf '%04d' $last_id
}
last_id() {
    last_id=$(get_last_dir)
    printf '%04d' $last_id
}

start_logging() {
    if [ $# -ge 1 ]
    then
	LOG_DIR="$LOG_DIR/$1"
    else
	LOG_DIR="$LOG_DIR/$(next_id)"
    fi
    mkdir -p $LOG_DIR

    record_git_information || warn "failed to record git information"
    clear_nginx_logfile || warn "failed to clear nginx logfile"
    # clear_mysql_logfile || warn "failed to clear mysql logfile"
    # start_dstat || warn "failed to start dstat"
    echo "Successfully started logging.  (Working directory: $LOG_DIR)"
}

record_git_information() {
    # record commit hash
    if ! git status >/dev/null 2>&1
    then
	# nothing to record (since logger.sh is not in any git repository)
	return 0
    fi
    CURRENT_COMMIT=$(git rev-parse HEAD)
    FILENAME=$(log_file_name_of git txt)
    date >>$FILENAME
    echo "git diff $CURRENT_COMMIT" >>$FILENAME
    git diff $CURRENT_COMMIT >>$FILENAME 2>&1
}
record_mysql_log() {
    cp $MYSQL_LOG_FILE $(log_file_name_of mysql-slow) || return 1
    chmod 644 $(log_file_name_of mysql-slow) || return 1
    mysqldumpslow -s t $(log_file_name_of mysql-slow) >$(log_file_name_of mysqldumpslow txt) || return 1
}
record_nginx_log() {
    cp $NGINX_LOG_FILE $(log_file_name_of nginx) || return 1
    chmod 644 $(log_file_name_of nginx) || return 1

    alp=$(go env GOPATH)/bin/alp
    if [ ! -x $alp ]
    then
	go get github.com/tkuchiki/alp || return 1
	[ -x $alp ] || return 1
    fi
    cat $(log_file_name_of nginx) | $alp --config "$ALP_CONF"  > "$(log_file_name_of nginxalp txt)" || return 1
}
record_cpuprof() {
    cp $CPUPROF_FILE $(log_file_name_of cpuprof) || return 1
    go tool pprof -list='main.*' $(log_file_name_of cpuprof) >$(log_file_name_of cpuproflist txt) || return 1
}
stop_logging() {
    if [ $# -ge 1 ]
    then
	LOG_DIR="$LOG_DIR/$1"
    else
	LOG_DIR="$LOG_DIR/$(last_id)"
    fi
    if [ ! -d $LOG_DIR ]
    then
	error "Could not find working directory: $LOG_DIR  (Did you started logging?)"
    fi

    bash ./tools/takelog.sh  # use old take log tool!
    cp ./tools/stat.txt $(log_file_name_of nginxstat txt)

    # record_mysql_log || warn "failed to record mysql log"
    # record_nginx_log || warn "failed to record nginx log"
    # stop_dstat || warn "failed to stop dstat"
    record_cpuprof || warn "failed to copy cpu profile"
    echo "Successfully stopped logging.  (Working directory: $LOG_DIR)"
}

terminate_logging() {
    if [ $# -eq 0 ]
    then
	error "Requires ID as argument"
    fi

    LOG_DIR="$LOG_DIR/$1"
    if [ ! -d $LOG_DIR ]
    then
	error "Could not find working directory: $LOG_DIR  (Did you started logging?)"
    fi

    stop_dstat || warn "failed to stop dstat"
    rm -r $LOG_DIR || error "Could not remove directory: $LOG_DIR"
    echo "Successfully terminated logging.  (Working directory: $LOG_DIR)"
}

main() {
    mkdir -p $LOG_DIR

    if [ $# -ge 1 ]
    then
	subcommand="$1"
	shift
    else
	subcommand=help
    fi

    case $subcommand in
	start)
	    start_logging $@
	    ;;
	stop)
	    stop_logging $@
	    ;;
	nextid)
	    next_id $@
	    ;;
	lastid)
	    last_id $@
	    ;;
	term)
	    terminate_logging $@
	    ;;
	help | -h | --help)
	    show_help_and_exit
	    ;;
	*)
	    echo "Unknown subcommand: $subcommand" >&2
	    show_help_and_exit 1
	    ;;
    esac
}

main $@
