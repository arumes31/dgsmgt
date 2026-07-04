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

	// Execute
	if err := writePacket(conn, 2, typeExecCommand, command); err != nil {
		return "", fmt.Errorf("rcon exec write: %w", err)
	}
	_, _, body, err := readPacket(conn)
	if err != nil {
		return "", fmt.Errorf("rcon exec read: %w", err)
	}
	return body, nil
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
