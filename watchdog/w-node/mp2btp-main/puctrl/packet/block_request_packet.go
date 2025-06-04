package packet

import (
	"bytes"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

const BLOCK_REQUEST_PACKET_HEADER_LEN = 20

type BlockRequestPacket struct {
	Type            byte
	Length          uint16
	BlockNumber     uint32
	StartDataNumber uint32
	EndDataNumber   uint32 // StartDataNumber ~ EndDataNumber (include EndDataNumber)
	MpInfo          byte   // For FEC Mode
	FecSeedNumber   uint32 // FEC Seed number
}

func CreateBlockRequestPacket(blockNumber uint32, startDataNumber uint32, endDataNumber uint32, mpInfo byte, fecSeedNumber uint32) *BlockRequestPacket {
	packet := BlockRequestPacket{}
	packet.Type = BLOCK_REQUEST_PACKET
	packet.Length = BLOCK_REQUEST_PACKET_HEADER_LEN
	packet.BlockNumber = blockNumber
	packet.StartDataNumber = startDataNumber
	packet.EndDataNumber = endDataNumber
	packet.MpInfo = mpInfo
	packet.FecSeedNumber = fecSeedNumber
	return &packet
}

func ParseBlockRequestPacket(r *bytes.Reader) (*BlockRequestPacket, error) {

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

	startDataNumber, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	endDataNumber, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	mpInfo, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	fecSeedNumber, err := util.ReadUint32(r)
	if err != nil {
		return nil, err
	}

	packet := &BlockRequestPacket{}
	packet.Type = packetType
	packet.Length = packetLegnth
	packet.BlockNumber = blockNumber
	packet.StartDataNumber = startDataNumber
	packet.EndDataNumber = endDataNumber
	packet.MpInfo = mpInfo
	packet.FecSeedNumber = fecSeedNumber

	return packet, nil
}

// Write Block Request Packet
func (p *BlockRequestPacket) Write(b *bytes.Buffer) error {
	b.WriteByte(p.Type)
	util.WriteUint16(b, uint16(p.Length))
	util.WriteUint32(b, uint32(p.BlockNumber))
	util.WriteUint32(b, uint32(p.StartDataNumber))
	util.WriteUint32(b, uint32(p.EndDataNumber))
	b.WriteByte(p.MpInfo)
	util.WriteUint32(b, uint32(p.FecSeedNumber))
	return nil
}
