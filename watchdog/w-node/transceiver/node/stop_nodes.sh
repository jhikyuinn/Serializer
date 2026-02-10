#!/bin/bash
set -euo pipefail

NODE_COUNT=20
MAX_JOBS=8   

stop_and_rm() {
  local name="consensus_node_$1"
  echo "Stopping+Removing $name"

  # 이미 멈췄거나 없을 수도 있으니 에러 무시
  docker stop -t 1 "$name" >/dev/null 2>&1 || true
  docker rm -f "$name" >/dev/null 2>&1 || true
}

running=0
for i in $(seq 1 "$NODE_COUNT"); do
  stop_and_rm "$i" &
  running=$((running+1))

  # 동시 실행 제한
  if [ "$running" -ge "$MAX_JOBS" ]; then
    wait -n
    running=$((running-1))
  fi
done

wait
echo "Done."


# wnode_0202_10kb
# wnode_0202_100kb
# wnode_0202_1mb
# wnode_0202_10mb
# wnode_0202_100mb

# wnodemp2btp_s_0202_10kb
# wnodemp2btp_s_0202_100kb
# wnodemp2btp_s_0202_1mb
# wnodemp2btp_s_0202_10mb
# wnodemp2btp_s_0202_100mb

# wnodemp2btp_m_0202_10kb
# wnodemp2btp_m_0202_100kb
# wnodemp2btp_m_0202_1mb
# wnodemp2btp_m_0202_10mb
# wnodemp2btp_m_0202_100mb