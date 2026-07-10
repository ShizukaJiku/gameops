package rcon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	typeAuth         = 3
	typeAuthResponse = 2
	typeCommand      = 2
	typeResponse     = 0
)

// commandID is the fixed request ID used for every Command() call. A Client
// is not safe for concurrent use (see Client's doc comment), so a single
// fixed ID is sufficient — no request is ever in flight concurrently with
// another.
const commandID = 2

// ErrAuthFailed is returned by Dial when the RCON password is rejected.
var ErrAuthFailed = errors.New("rcon: authentication failed")

// Client is a Source RCON protocol client. It is NOT safe for concurrent
// use — callers must serialize Command/Close calls (the current call sites
// in this project each do one Dial -> Command -> Close per operation,
// never sharing a Client across goroutines).
type Client struct {
	conn    net.Conn
	timeout time.Duration
}

// Dial connects to addr, authenticates with password, and returns a ready
// Client. Every subsequent Command call gets a fresh deadline of timeout,
// so a Command can never block past that window even if the server never
// responds.
func Dial(addr string, password string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("rcon: dial %s: %w", addr, err)
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("rcon: set deadline: %w", err)
	}
	c := &Client{conn: conn, timeout: timeout}
	if err := c.auth(password); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) auth(password string) error {
	if err := writePacket(c.conn, 1, typeAuth, password); err != nil {
		return fmt.Errorf("rcon: write auth: %w", err)
	}
	// Standard Source RCON sends an empty SERVERDATA_RESPONSE_VALUE before
	// the real auth response (id == request id on success, -1 on failure).
	// The vanilla Minecraft server skips that empty packet and replies with
	// SERVERDATA_AUTH_RESPONSE directly — accept both shapes.
	id, typ, _, err := readPacket(c.conn)
	if err != nil {
		return fmt.Errorf("rcon: read auth response: %w", err)
	}
	if typ == typeResponse {
		id, typ, _, err = readPacket(c.conn)
		if err != nil {
			return fmt.Errorf("rcon: read auth response: %w", err)
		}
	}
	if typ != typeAuthResponse {
		return fmt.Errorf("rcon: unexpected auth response packet type %d", typ)
	}
	if id == -1 {
		return ErrAuthFailed
	}
	return nil
}

// Command sends cmd and returns its response. Unlike standard Source RCON
// servers, Minecraft (vanilla and Forge) never splits a response across
// multiple packets — confirmed via manual testing against a live server,
// where sending the usual Source "empty sentinel packet" (used elsewhere to
// detect multi-packet response boundaries) caused the server to close the
// connection instead of replying. So Command reads exactly one packet back.
func (c *Client) Command(cmd string) (string, error) {
	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return "", fmt.Errorf("rcon: set deadline: %w", err)
	}
	if err := writePacket(c.conn, commandID, typeCommand, cmd); err != nil {
		return "", fmt.Errorf("rcon: write command: %w", err)
	}

	id, typ, body, err := readPacket(c.conn)
	if err != nil {
		return "", fmt.Errorf("rcon: read response: %w", err)
	}
	if typ != typeResponse {
		return "", fmt.Errorf("rcon: unexpected response packet type %d", typ)
	}
	if id != commandID {
		return "", fmt.Errorf("rcon: unexpected packet id %d", id)
	}
	return body, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func writePacket(w io.Writer, id int32, typ int32, body string) error {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, id); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.LittleEndian, typ); err != nil {
		return err
	}
	buf.WriteString(body)
	buf.WriteByte(0)
	buf.WriteByte(0)
	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, int32(buf.Len())); err != nil {
		return err
	}
	out.Write(buf.Bytes())
	_, err := w.Write(out.Bytes())
	return err
}

func readPacket(r io.Reader) (id int32, typ int32, body string, err error) {
	var length int32
	if err = binary.Read(r, binary.LittleEndian, &length); err != nil {
		return 0, 0, "", err
	}
	if length < 10 || length > 4096 {
		return 0, 0, "", fmt.Errorf("rcon: invalid packet length %d", length)
	}
	buf := make([]byte, length)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, 0, "", err
	}
	id = int32(binary.LittleEndian.Uint32(buf[0:4]))
	typ = int32(binary.LittleEndian.Uint32(buf[4:8]))
	body = string(bytes.TrimRight(buf[8:], "\x00"))
	return id, typ, body, nil
}
