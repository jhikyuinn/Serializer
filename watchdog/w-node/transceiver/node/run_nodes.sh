#!/bin/bash
set -euo pipefail

NODE_COUNT=4
MAX_JOBS=8

NET_A="net1"
NET_B="net2"
IMAGE="wnodemp2btp_m_0202_10kb"
ROUTER_NAME="router_net1_net2"

# ===== 고정 IP 계획 =====
# consensus_node_i:
#   net1(eth0): 172.21.0.i
#   net2(eth1): 172.22.0.i
# router:
#   net1: 172.21.0.6
#   net2: 172.22.0.6
SUBNET_A="172.21.0.0/24"
SUBNET_B="172.22.0.0/24"
GATEWAY_A="172.21.0.1"
GATEWAY_B="172.22.0.1"
ROUTER_IP_A="172.21.0.6"
ROUTER_IP_B="172.22.0.6"

install_ip_tools() {
  local name="$1"
  echo "Installing network tools in $name (if needed)..."

  if ! docker inspect "$name" >/dev/null 2>&1; then
    echo "ERROR: container $name does not exist"
    return 1
  fi

  if docker exec "$name" sh -c "command -v ip >/dev/null 2>&1"; then
    echo "ip already exists in $name"
  else
    if docker exec "$name" sh -c "command -v apk >/dev/null 2>&1"; then
      docker exec "$name" sh -c "apk add --no-cache tcpdump iproute2 iputils netcat-openbsd >/dev/null"
      return 0
    fi

    if docker exec "$name" sh -c "command -v apt-get >/dev/null 2>&1"; then
      docker exec "$name" sh -c "\
        DEBIAN_FRONTEND=noninteractive \
        apt-get update -y >/dev/null && \
        apt-get install -y iproute2 iputils-ping netcat tcpdump >/dev/null"
      return 0
    fi

    echo "ERROR: Cannot install network tools automatically in $name (no apk/apt-get)."
    return 1
  fi

  # ip는 있는데 ping/nc만 없을 수도 있어 보조 설치
  docker exec "$name" sh -c "command -v ping >/dev/null 2>&1 || \
    (command -v apk >/dev/null 2>&1 && apk add --no-cache iputils >/dev/null) || \
    (command -v apt-get >/dev/null 2>&1 && apt-get update -y >/dev/null && apt-get install -y iputils-ping >/dev/null)" || true

  docker exec "$name" sh -c "command -v nc >/dev/null 2>&1 || \
    (command -v apk >/dev/null 2>&1 && apk add --no-cache netcat-openbsd >/dev/null) || \
    (command -v apt-get >/dev/null 2>&1 && apt-get update -y >/dev/null && apt-get install -y netcat >/dev/null)" || true
}

stop_and_rm() {
  local name="$1"
  echo "Stopping+Removing $name"
  docker stop -t 1 "$name" >/dev/null 2>&1 || true
  docker rm -f "$name" >/dev/null 2>&1 || true
}

# ---- 0) 기존 노드/라우터 정리 ----
running=0
for i in $(seq 1 "$NODE_COUNT"); do
  stop_and_rm "consensus_node_$i" &
  running=$((running+1))
  if [ "$running" -ge "$MAX_JOBS" ]; then
    wait -n
    running=$((running-1))
  fi
done
stop_and_rm "$ROUTER_NAME" || true
wait
echo "Done."

echo "Waiting 2 seconds before starting containers..."
sleep 2

# ---- 1) 네트워크 재생성 (서브넷 지정: --ip 사용 가능) ----
# 기존 net1/net2가 "서브넷 없이" 만들어져 있으면 --ip가 불가하므로 삭제 후 재생성
docker network rm "$NET_A" >/dev/null 2>&1 || true
docker network rm "$NET_B" >/dev/null 2>&1 || true

docker network create --subnet "$SUBNET_A" --gateway "$GATEWAY_A" "$NET_A" >/dev/null
docker network create --subnet "$SUBNET_B" --gateway "$GATEWAY_B" "$NET_B" >/dev/null

echo "SUBNET_A=$SUBNET_A"
echo "SUBNET_B=$SUBNET_B"

# ---- 2) 노드 실행 (IP 고정) ----
run_node() {
  local i=$1
  local name="consensus_node_$i"
  local i=$(( $1 + 1 ))
  local ip_a="172.21.0.$i"
  local ip_b="172.22.0.$i"

  echo "Starting node $i ($name) with fixed IPs: $NET_A=$ip_a, $NET_B=$ip_b"

  docker run -d \
    --name "$name" \
    --privileged \
    --network "$NET_A" \
    --ip "$ip_a" \
    "$IMAGE" >/dev/null

  docker network connect --ip "$ip_b" "$NET_B" "$name"

  # 생성 검증 (실패 시 즉시 종료되게)
  docker inspect "$name" >/dev/null
}

running=0
for i in $(seq 1 "$NODE_COUNT"); do
  run_node "$i" &
  running=$((running+1))

  if [ "$running" -ge "$MAX_JOBS" ]; then
    wait -n
    running=$((running-1))
  fi
done
wait
echo "All nodes started."

for i in $(seq 1 "$NODE_COUNT"); do
  install_ip_tools "consensus_node_$i"
done

# ---- 3) 라우터 컨테이너 생성(net1 + net2) (IP 고정) ----
echo "Starting router container: $ROUTER_NAME with fixed IPs: $NET_A=$ROUTER_IP_A, $NET_B=$ROUTER_IP_B"
docker run -d \
  --name "$ROUTER_NAME" \
  --privileged \
  --network "$NET_A" \
  --ip "$ROUTER_IP_A" \
  alpine:3.19 sleep infinity >/dev/null

docker network connect --ip "$ROUTER_IP_B" "$NET_B" "$ROUTER_NAME"

# 라우터: iproute2 설치 + forwarding 켜기 + (테스트용) FORWARD ACCEPT
docker exec "$ROUTER_NAME" sh -c "apk add --no-cache iproute2 >/dev/null"
docker exec "$ROUTER_NAME" sh -c "sysctl -w net.ipv4.ip_forward=1 >/dev/null"
docker exec "$ROUTER_NAME" sh -c "iptables -P FORWARD ACCEPT 2>/dev/null || true"

echo "ROUTER_IP_A($NET_A)=$ROUTER_IP_A"
echo "ROUTER_IP_B($NET_B)=$ROUTER_IP_B"

# ---- 4) 각 노드에 양방향 policy routing 추가 ----
#  - from net1(eth0, 172.21.x.x): net2 -> via ROUTER_IP_A dev eth0 table 100
#  - from net2(eth1, 172.22.x.x): net1 -> via ROUTER_IP_B dev eth1 table 200
for i in $(seq 1 "$NODE_COUNT"); do
  name="consensus_node_$i"
  echo "Policy routing (bi-directional) in $name"

  docker exec "$name" sh -c "
    set -e

    # rp_filter off (멀티 NIC에서 필수인 경우 많음)
    sysctl -w net.ipv4.conf.all.rp_filter=0 >/dev/null
    sysctl -w net.ipv4.conf.default.rp_filter=0 >/dev/null
    sysctl -w net.ipv4.conf.eth0.rp_filter=0 >/dev/null 2>&1 || true
    sysctl -w net.ipv4.conf.eth1.rp_filter=0 >/dev/null 2>&1 || true

    NET1_IP=\$(ip -4 -o addr show dev eth0 | awk '{print \$4}' | cut -d/ -f1)
    NET2_IP=\$(ip -4 -o addr show dev eth1 | awk '{print \$4}' | cut -d/ -f1)

    # table 100: from net1 -> net2
    ip route del $SUBNET_B table 100 2>/dev/null || true
    ip route add $SUBNET_B via $ROUTER_IP_A dev eth0 table 100
    ip rule del priority 100 2>/dev/null || true
    ip rule del from \${NET1_IP}/32 table 100 2>/dev/null || true
    ip rule add priority 100 from \${NET1_IP}/32 table 100

    # table 200: from net2 -> net1
    ip route del $SUBNET_A table 200 2>/dev/null || true
    ip route add $SUBNET_A via $ROUTER_IP_B dev eth1 table 200
    ip rule del priority 200 2>/dev/null || true
    ip rule del from \${NET2_IP}/32 table 200 2>/dev/null || true
    ip rule add priority 200 from \${NET2_IP}/32 table 200

    ip route flush cache 2>/dev/null || true
  " || true
done

echo "Routing setup complete."

echo "Sanity check (IPs):"
for i in $(seq 1 "$NODE_COUNT"); do
  c="consensus_node_$i"
  echo "=== $c ==="
  docker exec -it "$c" sh -c "ip -4 -o addr show dev eth0 | awk '{print \"eth0:\",\$4}'"
  docker exec -it "$c" sh -c "ip -4 -o addr show dev eth1 | awk '{print \"eth1:\",\$4}'"
done

echo "Test example:"
echo "  docker exec -it consensus_node_1 sh -c 'ping -c 2 $ROUTER_IP_B'"
echo "  docker exec -it consensus_node_1 sh -c 'ip route get 172.21.0.2 from 172.22.0.1'"
