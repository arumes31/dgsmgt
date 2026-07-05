// Package rcon implements the Source RCON protocol (used by Minecraft,
// Source engine games, Rust, ARK and many others).
package rcon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const (
	typeAuth         = 3
	typeExecCommand  = 2
	typeAuthResponse = 2
	typeResponse     = 0

	execID          = 2
	sentinelID      = 7
	maxResponseSize = 1 << 20 // 1MB safety cap on assembled responses
)

// Send connects, authenticates and executes a single command, returning the
// server's response body.
var Send = func(host string, port int, password, command string) (string, error) {
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("rcon connect: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	// Authenticate
	if err := writePacket(conn, 1, typeAuth, password); err != nil {
		return "", fmt.Errorf("rcon auth write: %w", err)
	}
	for {
		id, ptype, _, err := readPacket(conn)
		if err != nil {
			return "", fmt.Errorf("rcon auth read: %w", err)
		}
		if ptype == typeAuthResponse {
			if id == -1 {
				return "", fmt.Errorf("rcon: invalid password")
			}
			break
		}
	}

	// Execute. Large responses are fragmented into multiple 4096-byte
	// packets (Source games' "status", Rust, ...). A sentinel exec sent
	// after the first full-size fragment marks the end — but it must go out
	// on its own: vanilla Minecraft reads one TCP segment at a time and
	// drops the connection when two packets coalesce into it.
	if err := writePacket(conn, execID, typeExecCommand, command); err != nil {
		return "", fmt.Errorf("rcon exec write: %w", err)
	}

	// Output keyed by the exec id is the command's reply; id-0 packets are
	// console broadcasts (Rust logs/chat) kept separately and used only when
	// the command id itself never yields output — so unrelated broadcast
	// lines can't pollute a real reply.
	var out, broadcast bytes.Buffer
	result := func() string {
		if out.Len() > 0 {
			return out.String()
		}
		return broadcast.String()
	}
	sentinelSent := false
	for {
		// Fresh budget per packet: once data flows, only inter-packet gaps
		// count, so servers that never echo the sentinel (Rust) cost at most
		// one short stall after the real response.
		wait := 10 * time.Second
		if out.Len() > 0 || broadcast.Len() > 0 {
			wait = 3 * time.Second
		}
		_ = conn.SetDeadline(time.Now().Add(wait))
		id, ptype, body, err := readPacket(conn)
		if err != nil {
			// Whatever arrived before the timeout is the response.
			if out.Len() > 0 || broadcast.Len() > 0 {
				return result(), nil
			}
			return "", fmt.Errorf("rcon exec read: %w", err)
		}
		if id == sentinelID {
			return result(), nil
		}
		if ptype != typeResponse {
			continue
		}
		switch id {
		case execID:
			out.WriteString(body)
			if out.Len() > maxResponseSize {
				return result(), nil
			}
			if !sentinelSent {
				// Bodies near the cap may be fragments — servers cap total
				// packet size at 4096 (body ~4086) or body at 4096, so probe
				// with a lone empty exec above ~4000 (sent separately: some
				// servers can't parse coalesced packets).
				if len(body) < 4000 {
					return result(), nil // small single packet: complete
				}
				sentinelSent = writePacket(conn, sentinelID, typeExecCommand, "") == nil
			}
		case 0:
			broadcast.WriteString(body)
			if broadcast.Len() > maxResponseSize {
				return result(), nil
			}
		}
	}
}

func writePacket(conn net.Conn, id int32, ptype int32, body string) error {
	var buf bytes.Buffer
	size := int32(4 + 4 + len(body) + 2)
	_ = binary.Write(&buf, binary.LittleEndian, size)
	_ = binary.Write(&buf, binary.LittleEndian, id)
	_ = binary.Write(&buf, binary.LittleEndian, ptype)
	buf.WriteString(body)
	buf.Write([]byte{0, 0})
	_, err := conn.Write(buf.Bytes())
	return err
}

func readPacket(conn net.Conn) (id int32, ptype int32, body string, err error) {
	var size int32
	if err = binary.Read(conn, binary.LittleEndian, &size); err != nil {
		return
	}
	if size < 10 || size > 4106 {
		err = fmt.Errorf("rcon: invalid packet size %d", size)
		return
	}
	data := make([]byte, size)
	if _, err = ioReadFull(conn, data); err != nil {
		return
	}
	id = int32(binary.LittleEndian.Uint32(data[0:4]))
	ptype = int32(binary.LittleEndian.Uint32(data[4:8]))
	if len(data) > 10 {
		body = string(data[8 : len(data)-2])
	}
	return
}

func ioReadFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
