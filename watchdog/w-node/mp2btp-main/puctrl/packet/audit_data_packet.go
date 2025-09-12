package packet

import (
	"bytes"

	"github.com/MCNL-HGU/mp2btp/puctrl/util"
)

type AuditDataPacket struct {
	Type      byte
	Length    uint16
	SessionID uint32

	Data []byte
}

func CreateAuditDataPacket(sessionID uint32, data []byte) *AuditDataPacket {
	packet := AuditDataPacket{}
	packet.Type = AUDIT_MSG_PACKET
	packet.Length = uint16(len(data))
	packet.SessionID = sessionID

	packet.Data = make([]byte, len(data))
	copy(packet.Data, data)

	return &packet
}

func ParseAuditDataPacket(r *bytes.Reader) (*AuditDataPacket, error) {

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

	packet := &AuditDataPacket{}
	packet.Type = packetType
	packet.Length = packetLegnth
	packet.SessionID = sessionID
	packet.Data = make([]byte, packetLegnth)
	r.Read(packet.Data)

	return packet, nil
}

// Write Block Data ACK Packet
func (p *AuditDataPacket) Write(b *bytes.Buffer) error {
	b.WriteByte(p.Type)
	util.WriteUint16(b, uint16(p.Length))
	util.WriteUint32(b, uint32(p.SessionID))
	b.Write(p.Data)

	return nil
}
