#!/bin/bash

# 사용법: ./log_node.sh 1
NODE_ID=$1

if [ -z "$NODE_ID" ]; then
  echo "사용법: $0 <node_number>"
  exit 1
fi

CONTAINER_NAME="consensus_node_$NODE_ID"

echo "📄 Logging container: $CONTAINER_NAME"
docker logs -f --tail 200 $CONTAINER_NAME