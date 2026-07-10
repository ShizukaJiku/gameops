package mcproto

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

// ReadPacket reads one length-prefixed Minecraft packet and returns its
// packet ID and remaining payload bytes.
func ReadPacket(r *bufio.Reader) (id int32, payload []byte, err error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if length < 0 {
		return 0, nil, fmt.Errorf("mcproto: negative length %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	pr := bufio.NewReader(bytes.NewReader(buf))
	id, err = ReadVarInt(pr)
	if err != nil {
		return 0, nil, err
	}
	rest, err := io.ReadAll(pr)
	if err != nil {
		return 0, nil, err
	}
	return id, rest, nil
}

// WritePacket frames id+data with a VarInt length prefix and writes it to w.
func WritePacket(w io.Writer, id int32, data []byte) error {
	body := append(WriteVarInt(id), data...)
	length := WriteVarInt(int32(len(body)))
	_, err := w.Write(append(length, body...))
	return err
}

// WriteString encodes a Minecraft protocol String (VarInt length + UTF-8 bytes).
func WriteString(s string) []byte {
	b := []byte(s)
	return append(WriteVarInt(int32(len(b))), b...)
}

// ReadString decodes a Minecraft protocol String from the start of payload,
// returning the string and the unconsumed remainder of payload.
func ReadString(payload []byte) (s string, rest []byte, err error) {
	r := bufio.NewReader(bytes.NewReader(payload))
	n, err := ReadVarInt(r)
	if err != nil {
		return "", nil, err
	}
	if n < 0 {
		return "", nil, fmt.Errorf("mcproto: negative length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", nil, err
	}
	rest, err = io.ReadAll(r)
	if err != nil {
		return "", nil, err
	}
	return string(buf), rest, nil
}
