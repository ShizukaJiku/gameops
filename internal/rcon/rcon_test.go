package rcon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeRconServer speaks just enough of the Source RCON protocol to test
// Client: accepts one connection, expects an AUTH packet with the given
// password, replies with the empty-response + auth-response pair, then for
// every SERVERDATA_EXECCOMMAND replies with the given command response.
func fakeRconServer(t *testing.T, password string, commandResponses map[string]string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		authID, authBody, _ := readTestPacket(conn)
		_ = authBody // password value not re-validated here; auth always succeeds
		writeTestPacket(conn, authID, typeResponse, "")
		writeTestPacket(conn, authID, typeAuthResponse, "")

		for {
			id, body, err := readTestPacket(conn)
			if err != nil {
				return
			}
			resp := commandResponses[body]
			writeTestPacket(conn, id, typeResponse, resp)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

func readTestPacket(r io.Reader) (id int32, body string, err error) {
	var length int32
	if err = binary.Read(r, binary.LittleEndian, &length); err != nil {
		return 0, "", err
	}
	buf := make([]byte, length)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, "", err
	}
	id = int32(binary.LittleEndian.Uint32(buf[0:4]))
	body = string(bytes.TrimRight(buf[8:], "\x00"))
	return id, body, nil
}

func writeTestPacket(w io.Writer, id int32, typ int32, body string) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, id)
	binary.Write(buf, binary.LittleEndian, typ)
	buf.WriteString(body)
	buf.WriteByte(0)
	buf.WriteByte(0)
	out := new(bytes.Buffer)
	binary.Write(out, binary.LittleEndian, int32(buf.Len()))
	out.Write(buf.Bytes())
	w.Write(out.Bytes())
}

func TestClientAuthAndCommand(t *testing.T) {
	addr := fakeRconServer(t, "secret", map[string]string{
		"list": "There are 0 of a max of 20 players online: ",
	})

	c, err := Dial(addr, "secret", 2*time.Second)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer c.Close()

	resp, err := c.Command("list")
	if err != nil {
		t.Fatalf("Command error: %v", err)
	}
	if resp != "There are 0 of a max of 20 players online: " {
		t.Fatalf("unexpected response: %q", resp)
	}
}

func TestCommandTimesOutInsteadOfHanging(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Complete auth normally, then go silent forever on any Command.
		authID, _, _ := readTestPacket(conn)
		writeTestPacket(conn, authID, typeResponse, "")
		writeTestPacket(conn, authID, typeAuthResponse, "")
		// Read the command packet(s) but never reply.
		for {
			if _, _, err := readTestPacket(conn); err != nil {
				return
			}
		}
	}()

	c, err := Dial(ln.Addr().String(), "secret", 300*time.Millisecond)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer c.Close()

	done := make(chan error, 1)
	go func() {
		_, err := c.Command("stop")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected Command to return an error when the server never responds")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Command did not return within 3s of its 300ms deadline — it hung")
	}
}

func TestDialAuthFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		authID, _, _ := readTestPacket(conn)
		writeTestPacket(conn, authID, typeResponse, "")
		writeTestPacket(conn, -1, typeAuthResponse, "") // auth rejected
	}()

	_, err = Dial(ln.Addr().String(), "wrong-password", 2*time.Second)
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expected errors.Is(err, ErrAuthFailed), got: %v", err)
	}
}

func TestDialAuthWrongAckType(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		authID, _, _ := readTestPacket(conn)
		const bogusType = 99
		writeTestPacket(conn, authID, bogusType, "")
	}()

	_, err = Dial(ln.Addr().String(), "secret", 2*time.Second)
	if err == nil {
		t.Fatal("expected Dial to return an error when server sends an unrecognized packet type")
	}
	if !strings.Contains(err.Error(), "unexpected auth response packet type") {
		t.Fatalf("expected error about unexpected auth response type, got: %v", err)
	}
}

// TestDialAuthMinecraftSingleAckPacket covers a real-world protocol
// deviation found via manual smoke testing against a live Forge server:
// unlike standard Source RCON servers, vanilla/Forge Minecraft does not
// send the leading empty SERVERDATA_RESPONSE_VALUE before the auth
// response — it replies with SERVERDATA_AUTH_RESPONSE directly.
func TestDialAuthMinecraftSingleAckPacket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		authID, _, _ := readTestPacket(conn)
		writeTestPacket(conn, authID, typeAuthResponse, "") // no leading empty packet
	}()

	c, err := Dial(ln.Addr().String(), "secret", 2*time.Second)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer c.Close()
}

// TestCommandRejectsMismatchedID covers the server sending back a response
// packet whose id doesn't match the command just sent.
func TestCommandRejectsMismatchedID(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		authID, _, _ := readTestPacket(conn)
		writeTestPacket(conn, authID, typeResponse, "")
		writeTestPacket(conn, authID, typeAuthResponse, "")

		cmdID, _, _ := readTestPacket(conn)
		writeTestPacket(conn, cmdID+1, typeResponse, "unexpected id")
	}()

	c, err := Dial(ln.Addr().String(), "secret", 2*time.Second)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer c.Close()

	if _, err := c.Command("list"); err == nil {
		t.Fatal("expected Command to return an error for a mismatched response id")
	}
}
