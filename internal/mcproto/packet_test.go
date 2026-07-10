package mcproto

import (
	"bufio"
	"bytes"
	"testing"
)

func TestWriteReadPacketRoundTrip(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := WritePacket(buf, 0x00, []byte("hello")); err != nil {
		t.Fatalf("WritePacket error: %v", err)
	}

	r := bufio.NewReader(buf)
	id, payload, err := ReadPacket(r)
	if err != nil {
		t.Fatalf("ReadPacket error: %v", err)
	}
	if id != 0x00 {
		t.Fatalf("expected id 0, got %d", id)
	}
	if string(payload) != "hello" {
		t.Fatalf("expected payload 'hello', got %q", payload)
	}
}

func TestStringRoundTrip(t *testing.T) {
	encoded := WriteString("test-string")
	s, rest, err := ReadString(encoded)
	if err != nil {
		t.Fatalf("ReadString error: %v", err)
	}
	if s != "test-string" {
		t.Fatalf("expected 'test-string', got %q", s)
	}
	if len(rest) != 0 {
		t.Fatalf("expected no leftover bytes, got %d", len(rest))
	}
}

func TestReadPacketNegativeLength(t *testing.T) {
	// Encode a negative length (-1) as a VarInt and try to read it as a packet
	buf := new(bytes.Buffer)
	buf.Write(WriteVarInt(-1))

	r := bufio.NewReader(buf)
	_, _, err := ReadPacket(r)
	if err == nil {
		t.Fatal("expected error for negative length, got nil")
	}
}

func TestReadStringNegativeLength(t *testing.T) {
	// Encode a negative length (-1) as a VarInt and try to read it as a string
	payload := WriteVarInt(-1)

	_, _, err := ReadString(payload)
	if err == nil {
		t.Fatal("expected error for negative length, got nil")
	}
}
