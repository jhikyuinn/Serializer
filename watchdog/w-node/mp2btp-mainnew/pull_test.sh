#!/bin/bash
# You must install sshpass 

# tmux session name 
SESSION="pull_test"

# SSH servers
servers=("yunmin@203.252.112.32" "mcnl@203.252.112.35" "mcnl@203.252.112.36" "mcnl@203.252.112.37")

echo -n "Enter your SSH password: "
read -s PASSWORD
echo 

# Start tmux sessions
tmux new-session -d -s $SESSION

# Start senders  
for ((i = 0; i < ${#servers[@]}; i++)); do
    INDEX=$((i+1))
    tmux new-window -t $SESSION:$INDEX
    tmux rename-window -t $SESSION:$INDEX "Sender$INDEX"
    tmux send-keys -t $SESSION:$INDEX "sshpass -p '$PASSWORD' ssh ${servers[i]} " C-m
    tmux send-keys -t $SESSION:$INDEX "cd ./research/mp2btp/example/pull/ && sudo ./pull_sender -i ${INDEX} -c pull_sender${INDEX}.toml" C-m
    sleep 0.5
    tmux send-keys -t $SESSION:$INDEX "$PASSWORD" C-m
done

# Start receiver at session 0
tmux send-keys -t $SESSION:0 "cd ./example/pull/ && sudo ./pull_receiver -i 0 -c pull_receiver0.toml" C-m
sleep 0.5
tmux send-keys -t $SESSION:0 "$PASSWORD" C-m

# Attach tmux session
tmux attach -t $SESSION