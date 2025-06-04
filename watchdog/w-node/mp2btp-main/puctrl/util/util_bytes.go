package util

import (
	"bytes"
	"io"
)

// Read reads a unsigned 16bits integer from r
func ReadUint16(r io.ByteReader) (uint16, error) {
	b1, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	b2, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	return uint16(b2) + uint16(b1)<<8, nil
}

// Read reads a unsigned 32bits integer from r
func ReadUint32(r io.ByteReader) (uint32, error) {
	b1, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	b2, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	b3, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	b4, err := r.ReadByte()
	if err != nil {
		return 0, err
	}

	return uint32(b4) + uint32(b3)<<8 + uint32(b2)<<16 + uint32(b1)<<24, nil
}

// Write uint32
func WriteUint32(w *bytes.Buffer, i uint32) {
	w.Write([]byte{uint8(i >> 24), uint8(i >> 16), uint8(i >> 8), uint8(i)})
}

// Write uint16
func WriteUint16(w *bytes.Buffer, i uint16) {
	w.Write([]byte{uint8(i >> 8), uint8(i)})
}
