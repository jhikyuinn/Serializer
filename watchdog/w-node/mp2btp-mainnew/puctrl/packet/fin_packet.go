package packet

import (
	"bytes"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

const FIN_PACKET_HEADER_LEN = 11

type FinPacket struct {
	Type        byte
	Length      uint16
	SessionID   uint32
	BlockNumber uint32
}

func CreateFinPacket(sessionID uint32, blockNumber uint32) *FinPacket {
	packet := FinPacket{}
	packet.Type = FIN_PACKET
	packet.Length = FIN_PACKET_HEADER_LEN
	packet.SessionID = sessionID
	packet.BlockNumber = blockNumber

	return &packet
}

func ParseFinPacket(r *bytes.Reader) (*FinPacket, error) {

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

	packet := &FinPacket{}
	packet.Type = packetType
	packet.Length = packetLegnth
	packet.SessionID = sessionID
	packet.BlockNumber = blockNumber

	return packet, nil
}

// Write FIN Packet
func (p *FinPacket) Write(b *bytes.Buffer) error {
	b.WriteByte(p.Type)
	util.WriteUint16(b, uint16(p.Length))
	util.WriteUint32(b, uint32(p.SessionID))
	util.WriteUint32(b, uint32(p.BlockNumber))

	return nil
}
