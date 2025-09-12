package packet

import (
	"bytes"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

const FIN_ACK_PACKET_HEADER_LEN = 11

type FinAckPacket struct {
	Type        byte
	Length      uint16
	SessionID   uint32
	BlockNumber uint32
}

func CreateFinAckPacket(sessionID uint32, blockNumber uint32) *FinAckPacket {
	packet := FinAckPacket{}
	packet.Type = FIN_ACK_PACKET
	packet.Length = FIN_ACK_PACKET_HEADER_LEN
	packet.SessionID = sessionID
	packet.BlockNumber = blockNumber

	return &packet
}

func ParseFinAckPacket(r *bytes.Reader) (*FinAckPacket, error) {

	packetType, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	packetLegnth, err := util.ReadUint16(r)
	if err != nil {
		return nil, err
	}

	sessionID, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	blockNumber, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	packet := &FinAckPacket{}
	packet.Type = packetType
	packet.Length = packetLegnth
	packet.SessionID = sessionID
	packet.BlockNumber = blockNumber

	return packet, nil
}

// Write FIN ACK Packet
func (p *FinAckPacket) Write(b *bytes.Buffer) error {
	b.WriteByte(p.Type)
	util.WriteUint16(b, uint16(p.Length))
	util.WriteUint32(b, uint32(p.SessionID))
	util.WriteUint32(b, uint32(p.BlockNumber))

	return nil
}
