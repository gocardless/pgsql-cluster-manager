#!/bin/bash

PG01="$1" # hostnames
PG02="$2"
PG03="$3"

cat <<EOF | crm configure
property stonith-enabled=false
property default-resource-stickiness=100
primitive Postgresql ocf:heartbeat:pgsql \
    params pgctl="/usr/lib/postgresql/9.4/bin/pg_ctl" psql="/usr/bin/psql" \
    pgdata="/var/lib/postgresql/9.4/main/" start_opt="-p 5432" rep_mode="sync" \
    node_list="$PG01 $PG02 $PG03" primary_conninfo_opt="keepalives_idle=60 keepalives_interval=5 \
    keepalives_count=5" repuser="postgres" tmpdir="/var/lib/postgresql/9.4/tmp" \
    config="/etc/postgresql/9.4/main/postgresql.conf" \
    logfile="/var/log/postgresql/postgresql-crm.log" restore_command="exit 0" \
    op start timeout="60s" interval="0s" on-fail="restart" \
    op monitor timeout="60s" interval="2s" on-fail="restart" \
    op monitor timeout="60s" interval="1s" on-fail="restart" role="Master" \
    op promote timeout="60s" interval="0s" on-fail="restart" \
    op demote timeout="60s" interval="0s" on-fail="stop" \
    op stop timeout="60s" interval="0s" on-fail="block" \
    op notify timeout="60s" interval="0s"
ms msPostgresql Postgresql params master-max=1 master-node-max=1 clone-max=3 clone-node-max=1 notify=true
commit
end
EOF
