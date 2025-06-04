package packet

import (
	"bytes"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

const PATH_INFO_PACKET_HEADER_LEN = 6

type PathInfoPacket struct {
	Type        byte
	Length      uint16
	SessionType uint16
	NumPath     byte
	IP          []uint32
}

func CreatePathInfoPacket(sessionType uint16, ipAddrs []string) *PathInfoPacket {
	packet := PathInfoPacket{}
	packet.Type = PATH_INFO_PACKET
	packet.Length = uint16(PATH_INFO_PACKET_HEADER_LEN + (len(ipAddrs) * 4))
	packet.SessionType = sessionType
	packet.NumPath = byte(len(ipAddrs))

	packet.IP = make([]uint32, len(ipAddrs))
	for i := 0; i < len(ipAddrs); i++ {
		packet.IP[i] = util.Ip2int(ipAddrs[i])
	}

	return &packet
}

func CreatePathInfoAckPacket(sessionType uint16, ipAddrs []string) *PathInfoPacket {
	packet := PathInfoPacket{}
	packet.Type = PATH_INFO_ACK_PACKET
	packet.Length = uint16(PATH_INFO_PACKET_HEADER_LEN + (len(ipAddrs) * 4))
	packet.SessionType = sessionType
	packet.NumPath = byte(len(ipAddrs))

	packet.IP = make([]uint32, len(ipAddrs))
	for i := 0; i < len(ipAddrs); i++ {
		packet.IP[i] = util.Ip2int(ipAddrs[i])
	}

	return &packet
}

func ParsePathInfoPacket(r *bytes.Reader) (*PathInfoPacket, error) {
	packetType, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	packetLegnth, err := util.ReadUint16(r)
	if err != nil {
		return nil, err
	}

	packetSessionType, err := util.ReadUint16(r)
	if err != nil {
		return nil, err
	}

	numPath, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	ipAddrs := make([]uint32, numPath)
	for i := 0; i < int(numPath); i++ {
		ipAddrs[i], err = util.ReadUint32(r)
		if err != nil {
			return nil, err
		}
	}

	packet := &PathInfoPacket{}
	packet.Type = packetType
	packet.Length = packetLegnth
	packet.SessionType = packetSessionType
	packet.NumPath = numPath
	packet.IP = ipAddrs

	return packet, nil
}

// Write Node Info Packet
func (p *PathInfoPacket) Write(b *bytes.Buffer) error {
	b.WriteByte(p.Type)
	util.WriteUint16(b, uint16(p.Length))
	util.WriteUint16(b, uint16(p.SessionType))
	b.WriteByte(p.NumPath)

	for i := 0; i < int(p.NumPath); i++ {
		util.WriteUint32(b, uint32(p.IP[i]))
	}

	return nil
}
