package mcproto

import (
	"bufio"
	"bytes"
	"testing"
)

func TestVarIntRoundTrip(t *testing.T) {
	cases := []int32{0, 1, 127, 128, 300, 2097151, 2147483647, -1}
	for _, v := range cases {
		encoded := WriteVarInt(v)
		r := bufio.NewReader(bytes.NewReader(encoded))
		got, err := ReadVarInt(r)
		if err != nil {
			t.Fatalf("ReadVarInt(%d) error: %v", v, err)
		}
		if got != v {
			t.Fatalf("round trip mismatch: sent %d got %d", v, got)
		}
	}
}
