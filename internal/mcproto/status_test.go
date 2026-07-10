package mcproto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
)

func buildHandshakePayload(protocolVersion int32, addr string, port uint16, nextState int32) []byte {
	buf := new(bytes.Buffer)
	buf.Write(WriteVarInt(protocolVersion))
	buf.Write(WriteString(addr))
	portBuf := []byte{byte(port >> 8), byte(port & 0xFF)}
	buf.Write(portBuf)
	buf.Write(WriteVarInt(nextState))
	return buf.Bytes()
}

func TestParseHandshakeStatus(t *testing.T) {
	payload := buildHandshakePayload(763, "example.com", 25565, 1)
	state, err := ParseHandshake(payload)
	if err != nil {
		t.Fatalf("ParseHandshake error: %v", err)
	}
	if state != 1 {
		t.Fatalf("expected next state 1, got %d", state)
	}
}

func TestParseHandshakeLogin(t *testing.T) {
	payload := buildHandshakePayload(763, "example.com", 25565, 2)
	state, err := ParseHandshake(payload)
	if err != nil {
		t.Fatalf("ParseHandshake error: %v", err)
	}
	if state != 2 {
		t.Fatalf("expected next state 2, got %d", state)
	}
}

func TestBuildStatusJSON(t *testing.T) {
	raw, err := BuildStatusJSON("sleeping", "", "")
	if err != nil {
		t.Fatalf("BuildStatusJSON error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	desc := parsed["description"].(map[string]any)
	if desc["text"] != "sleeping" {
		t.Fatalf("expected description.text 'sleeping', got %v", desc["text"])
	}
}

func TestBuildDisconnectJSON(t *testing.T) {
	raw, err := BuildDisconnectJSON("starting up")
	if err != nil {
		t.Fatalf("BuildDisconnectJSON error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["text"] != "starting up" {
		t.Fatalf("expected text 'starting up', got %v", parsed["text"])
	}
}

// sanity: buildHandshakePayload must itself be parseable with a bufio.Reader
// the same way ReadPacket would hand it to us.
func TestHandshakePayloadIsSelfConsistent(t *testing.T) {
	payload := buildHandshakePayload(763, "x", 1, 1)
	r := bufio.NewReader(bytes.NewReader(payload))
	if _, err := ReadVarInt(r); err != nil {
		t.Fatalf("protocol version not readable: %v", err)
	}
}

func TestParseHandshakeNegativeAddressLength(t *testing.T) {
	// construct a handshake payload with a negative address-length VarInt
	buf := new(bytes.Buffer)
	buf.Write(WriteVarInt(763)) // protocol version
	buf.Write(WriteVarInt(-1))  // negative address length (should fail)
	portBuf := []byte{byte(25565 >> 8), byte(25565 & 0xFF)}
	buf.Write(portBuf)
	buf.Write(WriteVarInt(1)) // next state

	payload := buf.Bytes()
	state, err := ParseHandshake(payload)
	if err == nil {
		t.Fatalf("ParseHandshake should reject negative address length, but succeeded with state %d", state)
	}
	// verify error message mentions negative length
	if err.Error() != "mcproto: negative length -1" {
		t.Fatalf("expected error 'mcproto: negative length -1', got '%v'", err)
	}
}

func TestBuildStatusJSONIncludesFavicon(t *testing.T) {
	raw, err := BuildStatusJSON("motd", "data:image/png;base64,AAAA", "")
	if err != nil {
		t.Fatalf("BuildStatusJSON error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["favicon"] != "data:image/png;base64,AAAA" {
		t.Fatalf("expected favicon field, got %v", parsed["favicon"])
	}
}

func TestBuildStatusJSONOmitsFaviconWhenEmpty(t *testing.T) {
	raw, err := BuildStatusJSON("motd", "", "")
	if err != nil {
		t.Fatalf("BuildStatusJSON error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := parsed["favicon"]; ok {
		t.Fatalf("expected favicon field to be omitted when empty, got %v", parsed["favicon"])
	}
}

func TestBuildStatusJSONVersionShowsLabelWithMismatchedProtocol(t *testing.T) {
	raw, err := BuildStatusJSON("motd", "", "⚡ Dormido")
	if err != nil {
		t.Fatalf("BuildStatusJSON error: %v", err)
	}
	var parsed struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Version.Name != "⚡ Dormido" {
		t.Fatalf("expected version.name %q, got %q", "⚡ Dormido", parsed.Version.Name)
	}
	// Any client's real protocol number is >= 0 and a small handful of
	// hundreds — 763 for 1.20.1. A negative number can never match a real
	// client, guaranteeing the "incompatible version" treatment (red X,
	// version.name shown in place of the ping bars) every time.
	if parsed.Version.Protocol >= 0 {
		t.Fatalf("expected a deliberately invalid (negative) protocol number to force version-mismatch display, got %d", parsed.Version.Protocol)
	}
}

func TestBuildStatusJSONKeepsRealVersionWhenLabelEmpty(t *testing.T) {
	raw, err := BuildStatusJSON("motd", "", "")
	if err != nil {
		t.Fatalf("BuildStatusJSON error: %v", err)
	}
	var parsed struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Version.Name != "1.20.1" || parsed.Version.Protocol != 763 {
		t.Fatalf("expected real version 1.20.1/763 when no label given, got %s/%d", parsed.Version.Name, parsed.Version.Protocol)
	}
}
