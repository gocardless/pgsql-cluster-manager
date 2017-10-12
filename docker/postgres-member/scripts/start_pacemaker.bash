#!/bin/bash

PG01="$1"
PG02="$2"
PG03="$3"

echo "Generating corosync config"
cat <<EOF > /etc/corosync/corosync.conf
totem {
      version: 2
      token: 3000
      token_retransmits_before_loss_const: 10
      join: 60
      consensus: 3600
      vsftype: none
      max_messages: 20
      clear_node_high_bit: yes
      secauth: on
      threads: 0

      rrp_mode: none
      interface {
              ringnumber: 0
              bindnetaddr: $(hostname -i)
              mcastport: 5405
      }
      transport: udpu
}

nodelist {
      node {
              ring0_addr: ${PG01}
              nodeid: 1
      }
      node {
              ring0_addr: ${PG02}
              nodeid: 2
      }
      node {
              ring0_addr: ${PG03}
              nodeid: 3
      }
}

amf {
      mode: disabled
}

quorum {
      provider: corosync_votequorum
}

aisexec {
      user:   root
      group:  root
}

logging {
      fileline: off
      to_stderr: yes
      to_logfile: yes
      to_syslog: no
      logfile: /var/log/corosync/corosync.log
      debug: off
      timestamp: on
      logger_subsys {
              subsys: AMF
              debug: off
              tags: enter|leave|trace1|trace2|trace3|trace4|trace6
      }
}
EOF

chown root:root /etc/corosync/corosync.conf
chmod 644 /etc/corosync/corosync.conf

echo "Starting corosync/pacemaker"
corosync # this starts corosync in the background
service pacemaker start
