# MP2BTP 

## Requirements

Go version >= 1.21.6 


## Compile

### Compile all modules and exmaples 

```sh
$ ./build.sh
```

clean 
```sh
$ ./build.sh clean 
```

### Compile Multipath Control (muctrl)

```sh
$ cd mp2btp/mulctrl
$ make 
```

### Compile Push/Pull Control (puctrl)

Push example
```sh
$ cd mp2btp/example/push
$ go build push_child.go
$ go build push_root.go
```

Pull example 
```sh
$ cd mp2btp/example/pull
$ go build pull_receiver.go
$ go build pull_sender.go
```

## Multi-path Setup 

### Packet forwarding
 
```sh
$ sudo sysctl -w net.ipv4.ip_forward=1
```

### Add iptables (TODO)

Iptables is automatically configured by the Puctrl module based on your configuration TOML file.

```sh
$ sudo iptables -t nat -A PREROUTING -p udp --dport 5000 -d 203.252.112.31 -j DNAT --to-destination 203.252.112.32:6000
$ sudo iptables -t nat -A OUTPUT -p udp --dport 5000 -d 203.252.112.31 -j DNAT --to-destination 203.252.112.32:6000
```

If you want delete iptables, 

```sh
$ sudo iptables -t nat -D PREROUTING -p udp --dport 5000 -d 203.252.112.31 -j DNAT --to-destination 203.252.112.32:6000
$ sudo iptables -t nat -D OUTPUT -p udp --dport 5000 -d 203.252.112.31 -j DNAT --to-destination 203.252.112.32:6000
```


## Test 
To run below shell script, you must install tmux and sshpass. 
Please refer to push_test.sh and pull_test.sh. 

### Push test 
```sh 
$ ./push_test.sh 
```

### Pull test 
```sh 
$ ./pull_test.sh 
```
