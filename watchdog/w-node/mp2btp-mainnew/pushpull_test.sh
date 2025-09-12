#!/bin/bash
# You must install sshpass 

# tmux session name 
SESSION="pushpull_test"

# SSH servers
servers=("yunmin@203.252.112.32" "mcnl@203.252.112.35" "mcnl@203.252.112.36" "mcnl@203.252.112.37")

echo -n "Enter your SSH password: "
read -s PASSWORD
echo 

# Start tmux sessions
tmux new-session -d -s $SESSION

# Start child nodes 
for ((i = ${#servers[@]}-1; i >= 0; i--)); do
    INDEX=$((i+1))
    tmux new-window -t $SESSION:$INDEX
    tmux rename-window -t $SESSION:$INDEX "Child$INDEX"
    tmux send-keys -t $SESSION:$INDEX "sshpass -p '$PASSWORD' ssh ${servers[i]} " C-m
    tmux send-keys -t $SESSION:$INDEX "cd ./research/mp2btp/example/push/ && sudo ./pushpull_child -i ${INDEX} -c push_child${INDEX}.toml" C-m
    sleep 0.5
    tmux send-keys -t $SESSION:$INDEX "$PASSWORD" C-m
done

sleep 1

# Start root node at session 0
tmux send-keys -t $SESSION:0 "cd ./example/push/ && sudo ./push_root -i 0 -c push_root.toml" C-m
sleep 0.5
tmux send-keys -t $SESSION:0 "$PASSWORD" C-m

# Attach tmux session
tmux attach -t $SESSION