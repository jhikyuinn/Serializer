package packet

import (
	"bytes"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

const NODE_INFO_PACKET_HEADER_LEN = 5

type NodeInfo struct {
	IP            uint32 // IP address excluding port number
	Port          uint16 // IP port
	NumOfChilds   byte   // # of child nodes
	OffsetOfChild byte   // offset of child
}

type NodeInfoPacket struct {
	Type        byte
	Length      uint16
	SessionType uint16

	NodeInfos []NodeInfo
}

func CreateNodeInfoPacket(sessionType uint16, NodeInfos []NodeInfo) *NodeInfoPacket {
	packet := NodeInfoPacket{}
	packet.Type = NODE_INFO_PACKET
	packet.Length = uint16(NODE_INFO_PACKET_HEADER_LEN + (len(NodeInfos) * 8))
	packet.SessionType = sessionType
	packet.NodeInfos = NodeInfos

	return &packet
}

func ParseNodeInfoPacket(r *bytes.Reader) (*NodeInfoPacket, error) {
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

	nodeInfosLen := (packetLegnth - NODE_INFO_PACKET_HEADER_LEN) / 8
	nodeInfos := make([]NodeInfo, nodeInfosLen)
	for i := 0; i < len(nodeInfos); i++ {
		nodeInfos[i].IP, err = util.ReadUint32(r)
		if err != nil {
			return nil, err
		}
		nodeInfos[i].Port, err = util.ReadUint16(r)
		if err != nil {
			return nil, err
		}
		nodeInfos[i].NumOfChilds, err = r.ReadByte()
		if err != nil {
			return nil, err
		}
		nodeInfos[i].OffsetOfChild, err = r.ReadByte()
		if err != nil {
			return nil, err
		}
	}

	packet := &NodeInfoPacket{}
	packet.Type = packetType
	packet.Length = packetLegnth
	packet.SessionType = packetSessionType
	packet.NodeInfos = nodeInfos

	return packet, nil
}

// Write Node Info Packet
func (p *NodeInfoPacket) Write(b *bytes.Buffer) error {
	b.WriteByte(p.Type)
	util.WriteUint16(b, uint16(p.Length))
	util.WriteUint16(b, uint16(p.SessionType))

	for i := 0; i < len(p.NodeInfos); i++ {
		util.WriteUint32(b, uint32(p.NodeInfos[i].IP))
		util.WriteUint16(b, uint16(p.NodeInfos[i].Port))
		b.WriteByte(p.NodeInfos[i].NumOfChilds)
		b.WriteByte(p.NodeInfos[i].OffsetOfChild)
	}

	return nil
}
