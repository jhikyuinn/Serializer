package packet

import (
	"bytes"
	"fmt"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

type BlockDataPacket struct {
	Type        byte
	Length      uint16
	BlockNumber uint32
	DataNumber  uint32
	SessionID   uint32
	DataType    byte

	Data []byte
}

const BLOCK_DATA_PACKET_HEADER_LEN = 16

func CreateBlockDataPacket(blockNumber uint32, dataNumber uint32, sessionID uint32, dataType byte, data []byte) *BlockDataPacket {
	packet := BlockDataPacket{}
	packet.Type = BLOCK_DATA_PACKET
	packet.Length = uint16(BLOCK_DATA_PACKET_HEADER_LEN + len(data))

	if packet.Length > BLOCK_DATA_PACKET_HEADER_LEN+PAYLOAD_SIZE {
		panic(fmt.Sprintf("packet length is larger than maximum size! (%d>%d)", packet.Length, BLOCK_DATA_PACKET_HEADER_LEN+PAYLOAD_SIZE))
	}

	packet.BlockNumber = blockNumber
	packet.DataNumber = dataNumber
	packet.SessionID = sessionID
	packet.DataType = dataType
	packet.Data = make([]byte, len(data))
	copy(packet.Data, data)

	return &packet
}

func ParseBlockDataPacket(r *bytes.Reader) (*BlockDataPacket, error) {

	packetType, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	packetLegnth, err := util.ReadUint16(r)
	if err != nil {
		return nil, err
	}

	blockNumber, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	dataNumber, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	sessionID, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	dataType, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	packet := &BlockDataPacket{}
	packet.Type = packetType
	packet.Length = packetLegnth
	packet.BlockNumber = blockNumber
	packet.DataNumber = dataNumber
	packet.SessionID = sessionID
	packet.DataType = dataType
	packet.Data = make([]byte, packetLegnth-BLOCK_DATA_PACKET_HEADER_LEN)
	r.Read(packet.Data)

	return packet, nil
}

// Write Block Data Packet
func (p *BlockDataPacket) Write(b *bytes.Buffer) error {
	b.WriteByte(p.Type)
	util.WriteUint16(b, uint16(p.Length))
	util.WriteUint32(b, uint32(p.BlockNumber))
	util.WriteUint32(b, uint32(p.DataNumber))
	util.WriteUint32(b, uint32(p.SessionID))
	b.WriteByte(p.DataType)
	b.Write(p.Data)

	return nil
}
