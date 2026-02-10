A_NET2=$(docker inspect -f '{{(index .NetworkSettings.Networks "net2").IPAddress}}' consensus_node_1)
B_NET2=$(docker inspect -f '{{(index .NetworkSettings.Networks "net2").IPAddress}}' consensus_node_2)

docker exec -it consensus_node_2 sh -c "ss -lunp | grep ':4000' || echo 'no UDP listener on 4000'"
docker exec -it consensus_node_1 sh -c "ss -lunp | grep ':4000' || echo 'no UDP listener on 4000'"


docker exec -it consensus_node_2 sh -c "tcpdump -ni any udp port 4000 -c 10"

docker exec -it consensus_node_1 sh -c "echo hi | nc -u -w 1 172.21.0.3 4000 || true"
