package tftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

type Packet interface {
	Unpack(data []byte) error
	Pack() []byte
}

const (
	OP_RRQ   = uint16(1) // Read request (RRQ)
	OP_WRQ   = uint16(2) // Write request (WRQ)
	OP_DATA  = uint16(3) // Data
	OP_ACK   = uint16(4) // Acknowledgement
	OP_ERROR = uint16(5) // Error
)

const (
	BLOCK_SIZE        = 512
	MAX_DATAGRAM_SIZE = 516
)

type RRQ struct {
	Filename string
	Mode     string
}

func (p *RRQ) Unpack(data []byte) (e error) {
	p.Filename, p.Mode, e = unpackRQ(data)
	if e != nil {
		return e
	}
	return nil
}

func (p *RRQ) Pack() []byte {
	return packRQ(p.Filename, p.Mode, OP_RRQ)
}

type WRQ struct {
	Filename string
	Mode     string
}

func (p *WRQ) Unpack(data []byte) (e error) {
	p.Filename, p.Mode, e = unpackRQ(data)
	if e != nil {
		return e
	}
	return nil
}

func (p *WRQ) Pack() []byte {
	return packRQ(p.Filename, p.Mode, OP_WRQ)
}

func unpackRQ(data []byte) (filename string, mode string, e error) {
	buffer := bytes.NewBuffer(data[2:])
	s, e := buffer.ReadString(0x0)
	if e != nil {
		return s, "", e
	}
	filename = strings.TrimSpace(strings.Trim(s, "\x00"))
	s, e = buffer.ReadString(0x0)
	if e != nil {
		return filename, s, e
	}
	mode = strings.TrimSpace(s)
	return filename, mode, nil
}

func packRQ(filename string, mode string, opcode uint16) []byte {
	buffer := &bytes.Buffer{}
	binary.Write(buffer, binary.BigEndian, opcode)
	buffer.WriteString(filename)
	buffer.WriteByte(0x0)
	buffer.WriteString(mode)
	buffer.WriteByte(0x0)
	return buffer.Bytes()
}

type DATA struct {
	BlockNumber uint16
	Data        []byte
}

func (p *DATA) Unpack(data []byte) (e error) {
	if len(data) < 4 { // DATA packet must have Opcode (2 bytes) and Block # (2 bytes)
		return fmt.Errorf("invalid DATA packet (length = %d)", len(data))
	}
	p.BlockNumber = binary.BigEndian.Uint16(data[2:])
	p.Data = data[4:]
	return nil
}

func (p *DATA) Pack() []byte {
	buffer := &bytes.Buffer{}
	binary.Write(buffer, binary.BigEndian, OP_DATA)
	binary.Write(buffer, binary.BigEndian, p.BlockNumber)
	buffer.Write(p.Data)
	return buffer.Bytes()
}

type ACK struct {
	BlockNumber uint16
}

func (p *ACK) Unpack(data []byte) (e error) {
	if len(data) < 4 { // ACK packet must have Opcode (2 bytes) and Block # (2 bytes)
		return fmt.Errorf("invalid ACK packet (length = %d)", len(data))
	}
	p.BlockNumber = binary.BigEndian.Uint16(data[2:])
	return nil
}

func (p *ACK) Pack() []byte {
	buffer := &bytes.Buffer{}
	binary.Write(buffer, binary.BigEndian, OP_ACK)
	binary.Write(buffer, binary.BigEndian, p.BlockNumber)
	return buffer.Bytes()
}

type ERROR struct {
	ErrorCode    uint16
	ErrorMessage string
}

func (p *ERROR) Unpack(data []byte) (e error) {
	p.ErrorCode = binary.BigEndian.Uint16(data[2:])
	if len(data) < 4 { // ACK packet must have Opcode (2 bytes) and ErrorCode (2 bytes)
		return fmt.Errorf("invalid ERROR packet (length = %d)", len(data))
	}
	buffer := bytes.NewBuffer(data[4:])
	s, e := buffer.ReadString(0x0)
	if e != nil {
		return e
	}
	p.ErrorMessage = strings.TrimSpace(s)
	return nil
}

func (p *ERROR) Pack() []byte {
	buffer := &bytes.Buffer{}
	binary.Write(buffer, binary.BigEndian, OP_ERROR)
	binary.Write(buffer, binary.BigEndian, p.ErrorCode)
	buffer.WriteString(p.ErrorMessage)
	buffer.WriteByte(0x0)
	return buffer.Bytes()
}

func ParsePacket(data []byte) (Packet, error) {
	var p Packet
	opcode := binary.BigEndian.Uint16(data)
	switch opcode {
	case OP_RRQ:
		p = &RRQ{}
	case OP_WRQ:
		p = &WRQ{}
	case OP_DATA:
		p = &DATA{}
	case OP_ACK:
		p = &ACK{}
	case OP_ERROR:
		p = &ERROR{}
	default:
		return nil, fmt.Errorf("unknown opcode: %d", opcode)
	}
	return p, p.Unpack(data)
}
