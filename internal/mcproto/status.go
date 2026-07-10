package mcproto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ParseHandshake extracts the "next state" field (1=status, 2=login) from a
// Handshake packet's payload. Protocol version, server address and port are
// consumed but not returned — the core only needs to know which state comes
// next.
func ParseHandshake(payload []byte) (nextState int32, err error) {
	r := bufio.NewReader(bytes.NewReader(payload))
	if _, err = ReadVarInt(r); err != nil { // protocol version
		return 0, err
	}
	strLen, err := ReadVarInt(r) // server address string length
	if err != nil {
		return 0, err
	}
	if strLen < 0 {
		return 0, fmt.Errorf("mcproto: negative length %d", strLen)
	}
	if _, err = io.CopyN(io.Discard, r, int64(strLen)); err != nil {
		return 0, err
	}
	if _, err = io.CopyN(io.Discard, r, 2); err != nil { // port, 2 bytes
		return 0, err
	}
	return ReadVarInt(r)
}

type statusResponse struct {
	Version     statusVersion `json:"version"`
	Players     statusPlayers `json:"players"`
	Description statusText    `json:"description"`
	Favicon     string        `json:"favicon,omitempty"`
}
type statusVersion struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}
type statusPlayers struct {
	Max    int `json:"max"`
	Online int `json:"online"`
}
type statusText struct {
	Text string `json:"text"`
}

// invalidProtocol is outside any real Minecraft protocol number range. A
// client comparing this against its own protocol always sees a mismatch,
// which makes it render the connect-compatibility indicator as an
// incompatible-version marker (red X) and show version.name in that spot
// instead of the usual ping bars — this is how versionLabel becomes visible
// without a hover, unlike the numeric players.online/max fields.
const invalidProtocol = -1

// BuildStatusJSON builds the JSON body for a Status Response packet, shown
// in the client's server list, with 0/0 players and a custom MOTD. favicon
// is a data URI ("data:image/png;base64,...") shown as the server's icon in
// the list, or "" to omit it. versionLabel, if non-empty, replaces the
// version name AND forces an invalid protocol number, surfacing state text
// (e.g. "⚡ Dormido") where the ping bars normally show. Empty versionLabel
// keeps the real version 1.20.1/763.
func BuildStatusJSON(motd string, favicon string, versionLabel string) (string, error) {
	version := statusVersion{Name: "1.20.1", Protocol: 763}
	if versionLabel != "" {
		version = statusVersion{Name: versionLabel, Protocol: invalidProtocol}
	}
	resp := statusResponse{
		Version:     version,
		Players:     statusPlayers{Max: 0, Online: 0},
		Description: statusText{Text: motd},
		Favicon:     favicon,
	}
	b, err := json.Marshal(resp)
	return string(b), err
}

// BuildDisconnectJSON builds the JSON chat-component body for a Disconnect
// packet sent during login.
func BuildDisconnectJSON(reason string) (string, error) {
	b, err := json.Marshal(statusText{Text: reason})
	return string(b), err
}
