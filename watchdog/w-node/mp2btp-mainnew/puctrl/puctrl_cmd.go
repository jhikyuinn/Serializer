package puctrl

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

// Disable forwarding - flush iptables
func DisablePacketForwarding() {
	// Generated commands
	//DOCKER
	cmd := "iptables -t nat --flush"
	// //LOCAL
	// cmd := "sudo iptables -w 5 -t nat --flush"
	cmdArgs := strings.Fields(cmd)
	util.Log("PuCtrl.DisablePacketForwarding(): %s", cmd)
	if err := RunCommand(cmdArgs[0], cmdArgs[1:]...); err != nil {
		util.Log("PuCtrl.DisablePacketForwarding(): Flush iptables Failed!")
	} else {
		util.Log("PuCtrl.DisablePacketForwarding(): Flush iptables Succeeded!")
	}
}

// Enable forwarding
func EnablePacketForwarding(index uint16, isPullSender bool) {
	// Generated commands
	// //DOCKER
	cmdFmt := []string{
		"iptables -t nat %s PREROUTING -p udp --sport %d -s %s -j DNAT --to-destination %s:%d",
		"iptables -t nat %s OUTPUT -p udp --sport %d -s %s -j DNAT --to-destination %s:%d",
	}
	// //LOCAL
	// cmdFmt := []string{
	// 	"sudo iptables -w 5 -t nat %s PREROUTING -p udp --sport %d -s %s -j DNAT --to-destination %s:%d",
	// 	"sudo iptables -w 5 -t nat %s OUTPUT -p udp --sport %d -s %s -j DNAT --to-destination %s:%d",
	// }

	cmd := make([]string, 2)

	// Delete forwarding rule
	if !isPullSender {
		cmd[0] = fmt.Sprintf(cmdFmt[0], "-D", Conf.EQUIC_APP_DST_PORT+index, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
		cmd[1] = fmt.Sprintf(cmdFmt[1], "-D", Conf.EQUIC_APP_DST_PORT+index, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
	} else {
		cmd[0] = fmt.Sprintf(cmdFmt[0], "-D", Conf.LISTEN_PORT, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
		cmd[1] = fmt.Sprintf(cmdFmt[1], "-D", Conf.LISTEN_PORT, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
	}

	for _, c := range cmd {
		cmdArgs := strings.Fields(c)
		util.Log("PuCtrl.EnablePacketForwarding(): %s", c)
		if err := RunCommand(cmdArgs[0], cmdArgs[1:]...); err != nil {
			util.Log("PuCtrl.EnablePacketForwarding(): Delete Rule Failed!")
		} else {
			util.Log("PuCtrl.EnablePacketForwarding(): Delete Rule Succeeded!")
		}
	}

	// Append forwarding rule
	if !isPullSender {
		cmd[0] = fmt.Sprintf(cmdFmt[0], "-A", Conf.EQUIC_APP_DST_PORT+index, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
		cmd[1] = fmt.Sprintf(cmdFmt[1], "-A", Conf.EQUIC_APP_DST_PORT+index, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
	} else {
		cmd[0] = fmt.Sprintf(cmdFmt[0], "-A", Conf.LISTEN_PORT, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
		cmd[1] = fmt.Sprintf(cmdFmt[1], "-A", Conf.LISTEN_PORT, Conf.MY_IP_ADDRS[0], Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)
	}

	for _, c := range cmd {
		cmdArgs := strings.Fields(c)
		util.Log("PuCtrl.EnablePacketForwarding(): %s", c)
		if err := RunCommand(cmdArgs[0], cmdArgs[1:]...); err != nil {
			panic("PuCtrl.EnablePacketForwarding(): Append Rule Failed!")
		} else {
			util.Log("PuCtrl.EnablePacketForwarding(): Append Rule Succeeded!")
		}
	}
}

// Run Enhanced QUIC by creating process
// func (s *PuSession) RunEnhancedQuic(dstIpAddr []uint32, isReceiver bool, index uint16) {
// 	cmd := ""

// 	// Generate fectun options
// 	// FEC mode
// 	if Conf.EQUIC_FEC_MODE {
// 		cmd += "-f "
// 	}

// 	// receiver
// 	if isReceiver {
// 		cmd += "-r "
// 	}

// 	// application source address (-a option)
// 	cmd += fmt.Sprintf("-a %s:%d ", Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)

// 	// application destination address (-u option)
// 	if isReceiver && s.sessionType == PUSH_SESSION ||
// 		!isReceiver && s.sessionType == PULL_SESSION {
// 		cmd += fmt.Sprintf("-u %s:%d ", Conf.MY_IP_ADDRS[0], Conf.LISTEN_PORT)
// 	} else {
// 		cmd += fmt.Sprintf("-u %s:%d ", Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_DST_PORT+index)
// 	}

// 	// tunnel source address (-s option)
// 	for i := 0; i < int(Conf.NUM_MULTIPATH); i++ {
// 		cmd += fmt.Sprintf("-s %s:%d ", Conf.MY_IP_ADDRS[i], Conf.EQUIC_TUN_SRC_PORT+index)
// 	}

// 	// tunnel destination address (-d option)
// 	if !isReceiver && s.sessionType == PULL_SESSION {
// 		for i := 0; i < len(dstIpAddr); i++ {
// 			cmd += fmt.Sprintf("-d %s:%d ", util.Int2ip(dstIpAddr[i]), Conf.EQUIC_TUN_DST_PORT+uint16(s.puCtrl.peerId)-1) // Tricky
// 		}
// 	} else {
// 		for i := 0; i < len(dstIpAddr); i++ {
// 			cmd += fmt.Sprintf("-d %s:%d ", util.Int2ip(dstIpAddr[i]), Conf.EQUIC_TUN_DST_PORT)
// 		}
// 	}

// 	util.Log("PuCtrl.StartEnhancedQuic(): %s", cmd)
// 	cmdArgs := strings.Fields(cmd)

// 	if err := RunCommand(Conf.EQUIC_PATH, cmdArgs[0:]...); err != nil {
// 		panic("PuCtrl.StartEnhancedQuic(): Enhanced QUIC Failed!")
// 	}
// 	util.Log("PuCtrl.StartEnhancedQuic(): Enhanced QUIC Started!")
// }

func RunEnhancedQuic(dstIpAddr []string, isReceiver bool, sessionType int, index uint16) {
	cmd := ""

	// Generate fectun options
	// FEC mode
	if Conf.EQUIC_FEC_MODE {
		cmd += "-f "
	}

	// receiver
	if isReceiver {
		cmd += "-r "
	}

	// application source address (-a option)
	cmd += fmt.Sprintf("-a %s:%d ", Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_SRC_PORT+index)

	// application destination address (-u option)
	if (isReceiver && sessionType == PUSH_SESSION) ||
		(!isReceiver && sessionType == PULL_SESSION) {
		cmd += fmt.Sprintf("-u %s:%d ", Conf.MY_IP_ADDRS[0], Conf.LISTEN_PORT)
	} else {
		cmd += fmt.Sprintf("-u %s:%d ", Conf.MY_IP_ADDRS[0], Conf.EQUIC_APP_DST_PORT+index)
	}

	// tunnel source address (-s option)
	for i := 0; i < int(Conf.NUM_MULTIPATH); i++ {
		cmd += fmt.Sprintf("-s %s:%d ", Conf.MY_IP_ADDRS[i], Conf.EQUIC_TUN_SRC_PORT+index)
	}

	// tunnel destination address (-d option)
	if (dstIpAddr != nil && len(dstIpAddr) > 0) && !isReceiver {
		if !isReceiver && sessionType == PULL_SESSION {
			for i := 0; i < len(dstIpAddr); i++ {
				//cmd += fmt.Sprintf("-d %s:%d ", util.Int2ip(dstIpAddr[i]), Conf.EQUIC_TUN_DST_PORT+uint16(0)-1) // Tricky
				cmd += fmt.Sprintf("-d %s:%d ", dstIpAddr[i], Conf.EQUIC_TUN_DST_PORT+uint16(0)) // Tricky
			}
		} else {
			for i := 0; i < len(dstIpAddr); i++ {
				//cmd += fmt.Sprintf("-d %s:%d ", util.Int2ip(dstIpAddr[i]), Conf.EQUIC_TUN_DST_PORT)
				cmd += fmt.Sprintf("-d %s:%d ", dstIpAddr[i], Conf.EQUIC_TUN_DST_PORT)
			}
		}
	}

	// to be modified
	if (dstIpAddr != nil && len(dstIpAddr) > 0) && isReceiver && sessionType == PULL_SESSION {
		for i := 0; i < len(dstIpAddr); i++ {
			cmd += fmt.Sprintf("-d %s:%d ", dstIpAddr[i], Conf.EQUIC_TUN_DST_PORT) // Tricky
		}
	}

	util.Log("PuCtrl.StartEnhancedQuic(): %s", cmd)
	cmdArgs := strings.Fields(cmd)

	if err := RunCommand(Conf.EQUIC_PATH, cmdArgs[0:]...); err != nil {
		fmt.Println(err)
		// panic("PuCtrl.StartEnhancedQuic(): Enhanced QUIC Failed!")
	}
	util.Log("PuCtrl.StartEnhancedQuic(): Enhanced QUIC Started!")
}

func RunCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout //nil
	cmd.Stderr = os.Stderr //nil
	err := cmd.Run()
	return err
	// return nil
}
