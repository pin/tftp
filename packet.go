package tftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	opRRQ   = uint16(1) // Read request (RRQ)
	opWRQ   = uint16(2) // Write request (WRQ)
	opDATA  = uint16(3) // Data
	opACK   = uint16(4) // Acknowledgement
	opERROR = uint16(5) // Error
	opOACK  = uint16(6) // Options Acknowledgment
)

const (
	blockLength    = 512
	datagramLength = 516
)

// RRQ/WRQ packet
//
//  2 bytes     string    1 byte    string    1 byte
// --------------------------------------------------
// | Opcode |  Filename  |   0  |    Mode    |   0  |
// --------------------------------------------------
type pRRQ []byte
type pWRQ []byte

// packRQ returns length of the packet in b
func packRQ(b []byte, op uint16, filename, mode string) int {
	binary.BigEndian.PutUint16(b, op)
	n := copy(b[2:len(b)-10], filename)
	b[2+n] = 0
	m := copy(b[3+n:len(b)-2], mode)
	b[2+n+1+m] = 0
	return 2 + n + 1 + m + 1
}

func unpackRQ(p []byte) (filename, mode string, err error) {
	buffer := bytes.NewBuffer(p[2:])
	s, err := buffer.ReadString(0x0)
	if err != nil {
		return s, "", err
	}
	filename = strings.TrimSpace(strings.Trim(s, "\x00"))
	s, err = buffer.ReadString(0x0)
	if err != nil {
		return filename, s, err
	}
	mode = strings.TrimSpace(strings.Trim(s, "\x00"))
	return filename, mode, nil
}

// ERROR packet
//
//  2 bytes     2 bytes      string    1 byte
// ------------------------------------------
// | Opcode |  ErrorCode |   ErrMsg   |  0  |
// ------------------------------------------
type pERROR []byte

func packERROR(p []byte, code uint16, message string) int {
	binary.BigEndian.PutUint16(p, opERROR)
	binary.BigEndian.PutUint16(p[2:], code)
	n := copy(p[4:len(p)-2], message)
	p[4+n] = 0
	return n + 5
}

func (p pERROR) code() uint16 {
	return binary.BigEndian.Uint16(p[2:])
}

func (p pERROR) message() string {
	return string(p[4:])
}

// DATA packet
//
//  2 bytes    2 bytes     n bytes
// ----------------------------------
// | Opcode |   Block #  |   Data   |
// ----------------------------------
type pDATA []byte

func (p pDATA) block() uint16 {
	return binary.BigEndian.Uint16(p[2:])
}

// ACK packet
//
//  2 bytes    2 bytes
// -----------------------
// | Opcode |   Block #  |
// -----------------------
type pACK []byte

func (p pACK) block() uint16 {
	return binary.BigEndian.Uint16(p[2:])
}

func parsePacket(p []byte) (interface{}, error) {
	l := len(p)
	if l < 2 {
		return nil, fmt.Errorf("short packet")
	}
	opcode := binary.BigEndian.Uint16(p)
	switch opcode {
	case opRRQ:
		if l < 4 {
			return nil, fmt.Errorf("short RRQ packet: %d", l)
		}
		return pRRQ(p), nil
	case opWRQ:
		if l < 4 {
			return nil, fmt.Errorf("short WRQ packet: %d", l)
		}
		return pWRQ(p), nil
	case opDATA:
		if l < 4 {
			return nil, fmt.Errorf("short DATA packet: %d", l)
		}
		return pDATA(p), nil
	case opACK:
		if l < 4 {
			return nil, fmt.Errorf("short ACK packet: %d", l)
		}
		return pACK(p), nil
	case opERROR:
		if l < 5 {
			return nil, fmt.Errorf("short ERROR packet: %d", l)
		}
		return pERROR(p), nil
	default:
		return nil, fmt.Errorf("unknown opcode: %d", opcode)
	}
}
