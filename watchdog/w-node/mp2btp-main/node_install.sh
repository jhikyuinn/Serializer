#!/bin/bash
# You must install sshpass 

FILE="../account.txt"
SESSION="node_install"

# SSH Servers
servers=("yunmin@203.252.112.32" "mcnl@203.252.112.35" "mcnl@203.252.112.36" "mcnl@203.252.112.37")

COMMIT_MESSAGE=$1

# Usage
if [ -z "$COMMIT_MESSAGE" ]; then
  echo "Usage: $0 <github commit message>"
  exit 1
fi

# SSH password 
echo -n "Enter your SSH password: "
read -s PASSWORD
echo 

if [[ ! -f $FILE ]]; then
    echo "File not found! $FILE"
    exit 1
fi 

#echo -n "Enter your Github email: "
#read GITHUB_EMAIL

#echo -n "Enter your Github Password: "
#read GITHUB_PASSWORD
#echo 

GITHUB_EMAIL=$(sed -n '1p' $FILE)
GITHUB_PASSWORD=$(sed -n '2p' $FILE)

echo "Github EMAIL: $GITHUB_EMAIL"
echo "Github Password: $GITHUB_PASSWORD"

# Git commit 
./build.sh clean
git status
git add .
git commit -m $COMMIT_MESSAGE
git push origin main

# Start new session 
tmux new-session -d -s $SESSION

# Install 
for ((i = 0; i < ${#servers[@]}; i++)); do
    INDEX=$((i+1))
    tmux new-window -t $SESSION:$INDEX
    tmux rename-window -t $SESSION:$INDEX "Sender$INDEX"
    tmux send-keys -t $SESSION:$INDEX "sshpass -p '$PASSWORD' ssh ${servers[i]} " C-m
    tmux send-keys -t $SESSION:$INDEX "cd ./research/mp2btp && ./build.sh clean " C-m 
    tmux send-keys -t $SESSION:$INDEX "git fetch --all && git reset --hard origin/main" C-m 
    sleep 0.5
    tmux send-keys -t $SESSION:$INDEX "$GITHUB_EMAIL" C-m
    sleep 0.5
    tmux send-keys -t $SESSION:$INDEX "$GITHUB_PASSWORD" C-m
    sleep 0.5 
    tmux send-keys -t $SESSION:$INDEX "./build.sh" C-m
done

# Run build.sh at session 0
tmux send-keys -t $SESSION:0 "./build.sh" C-m 

# Attach tmux 
tmux attach -t $SESSION
