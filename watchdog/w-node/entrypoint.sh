#!/bin/bash

# mongod를 백그라운드가 아닌 포그라운드로 실행
mongod --dbpath /data/db --bind_ip_all --fork --logpath /var/log/mongod.log

MY_IP=$(hostname -I | awk '{print $1}')

# TOML 설정 생성
cat <<EOF > /app/push_child.toml
VERBOSE_MODE = true

NUM_MULTIPATH = 1
MY_IP_ADDRS = ["$MY_IP","127.0.0.1"]
LISTEN_PORT = 4000
BIND_PORT = 5000

EQUIC_ENABLE = true
EQUIC_PATH = "./fectun"
EQUIC_APP_SRC_PORT = 6000
EQUIC_APP_DST_PORT = 5000
EQUIC_TUN_SRC_PORT = 7000
EQUIC_TUN_DST_PORT = 7000
EQUIC_FEC_MODE = false

SEGMENT_SIZE = 2097152
FEC_SEGMENT_SIZE = 2466816

THROUGHPUT_PERIOD = 0.1
THROUGHPUT_WEIGHT = 0.1
MULTIPATH_THRESHOLD_THROUGHPUT = 1000.0
MULTIPATH_THRESHOLD_SEND_COMPLETE_TIME = 0.0
CLOSE_TIMEOUT_PERIOD = 200
EOF

# 이후 wnode 실행
./wnode