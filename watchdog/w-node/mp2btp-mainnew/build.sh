#!/bin/bash
cd mulctrl || { echo "No 'mulctrl' directory"; exit 1; }

if [ "$1" == "clean" ]; then
    # Clean 
    echo "Remove mulctrl: make clean"
    make clean > /dev/null 2>&1
    cd ../example/push
    echo "Remove puctrl/test/push" 
    rm -f push_root push_child pushpull_child gmon.out peer_*block* *.txt
    cd ../pull
    echo "Remove puctrl/test/pull" 
    rm -f pull_receiver pull_sender gmon.out peer_*block* *.txt
    cd ../../
else
    # Build
    echo "Build mulctrl: make"
    make clean > /dev/null 2>&1
    make > /dev/null 2>&1
    cd ../example/push
    echo "Build example/push: go build push_root.go; go build push_child.go; go build pushpull_child.go" 
    go build push_root.go; go build push_child.go; go build pushpull_child.go 
    cd ../pull
    echo "Build example/pull: go build pull_receiver.go; go build pull_sender.go" 
    go build pull_receiver.go; go build pull_sender.go     
    cd ../../
fi