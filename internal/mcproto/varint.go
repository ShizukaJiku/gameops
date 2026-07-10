package mcproto

import (
	"bufio"
	"errors"
)

func ReadVarInt(r *bufio.Reader) (int32, error) {
	var value int32
	var position uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= int32(b&0x7F) << position
		if b&0x80 == 0 {
			break
		}
		position += 7
		if position >= 32 {
			return 0, errors.New("mcproto: varint too big")
		}
	}
	return value, nil
}

func WriteVarInt(value int32) []byte {
	var buf []byte
	uv := uint32(value)
	for {
		b := byte(uv & 0x7F)
		uv >>= 7
		if uv != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if uv == 0 {
			break
		}
	}
	return buf
}
