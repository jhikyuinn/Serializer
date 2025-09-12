package puctrl

var Conf Config

type PeerAddr struct {
	Addr        string `toml:"Addr"`
	NumChild    byte   `toml:"NumChild"`
	ChildOffset byte   `toml:"ChildOffset"`
}

type Config struct {
	CONFIG_USER_WRR_WEIGHT []uint32
	VERBOSE_MODE           bool
	NUM_MULTIPATH          uint32
	MY_IP_ADDRS            []string
	PEER_ADDRS             []PeerAddr `toml:"peer_addr"`

	// Multi-Peer with FEC Mode
	MULTIPEER_FEC_MODE bool

	LISTEN_PORT uint16
	BIND_PORT   uint16

	// Enhanced QUIC - FEC Tunneling
	EQUIC_ENABLE       bool
	EQUIC_PATH         string
	EQUIC_APP_SRC_PORT uint16
	EQUIC_APP_DST_PORT uint16
	EQUIC_TUN_SRC_PORT uint16
	EQUIC_TUN_DST_PORT uint16
	EQUIC_FEC_MODE     bool

	PULL_MODE        byte
	NUM_PEERS        uint16
	SEGMENT_SIZE     uint32
	FEC_SEGMENT_SIZE uint32

	THROUGHPUT_PERIOD                      float32
	THROUGHPUT_WEIGHT                      float32
	MULTIPATH_THRESHOLD_THROUGHPUT         float64
	MULTIPATH_THRESHOLD_SEND_COMPLETE_TIME float64
	CLOSE_TIMEOUT_PERIOD                   uint16
	PUSH_PULL_TRANSITION_TIMEOUT           uint16 // Added
	MAX_CONNECTION_RETRY                   uint16 // Added
	FAULT_PEER                             int    // For push-to-pull transition experiment

}

const (
	PUSH_SESSION = 0
	PULL_SESSION = 1

	PEER_NOT_KNOWN    = 0
	PEER_HAS_BLOCK    = 1
	PEER_HAS_NO_BLOCK = 2
	PEER_DOWNLOADING  = 3

	DATA_TYPE_SOURCE = 0
	DATA_TYPE_FEC    = 1

	TIMEOUT = 2

	PULL_MODE_SP     = 0 // Receiving single segment from Single Peer (without FEC)
	PULL_MODE_MP     = 1 // Receiving single segment from Multiple Peer (without FEC)
	PULL_MODE_MP_FEC = 2 // Receiving single segment from Multiple Peer using FEC

	WAIT_FIN     = 0
	WAIT_FIN_ACK = 1
)
