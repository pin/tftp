package tftp

import (
	"net"
	"fmt"
	"io"
)

type Server struct {
	BindAddr *net.UDPAddr
	ReadHandler func(filename string, r *io.PipeReader)
	WriteHandler func(filename string, w *io.PipeWriter)
}

func (s Server) Serve() (error) {
	conn, e := net.ListenUDP("udp", s.BindAddr)
	if e != nil {
		return fmt.Errorf("Failed listen UDP %v", e)
	}
	for {
		_ = s.processRequest(conn)
	}
}

func (s Server) processRequest(conn *net.UDPConn) (error) {
	var buffer []byte
	buffer = make([]byte, 50)
	
	written, addr, e := conn.ReadFromUDP(buffer)
	if e != nil {
		return fmt.Errorf("Failed to read data from client: %v", e)
	}
	p, e := ParsePacket(buffer[:written])
	
	switch p := Packet(*p).(type) {
		case *WRQ:
			trasnmissionConn, e := s.transmissionConn()
			if e != nil {
				return fmt.Errorf("could not start transmission: %v", e)
			}
			reader, writer := io.Pipe()
			r := &Receiver{addr, trasnmissionConn, writer}
			fmt.Printf("write %s\n", p.Filename)
			go s.ReadHandler(p.Filename, reader)
			go r.Run()
		case *RRQ:
			trasnmissionConn, e := s.transmissionConn()
			if e != nil {
				return fmt.Errorf("could not start transmission: %v", e)
			}
			reader, writer := io.Pipe()
			r := &Sender{addr, trasnmissionConn, reader}
			fmt.Printf("read %s\n", p.Filename)
			go s.WriteHandler(p.Filename, writer)
			go r.Run()
	}
	return nil
}

func (s Server) transmissionConn() (*net.UDPConn, error) {
	addr, e := net.ResolveUDPAddr("udp", ":0")
	if e != nil {
		return nil, e
	}
	conn, e := net.ListenUDP("udp", addr)
	if e != nil {
		return nil, e
	}
	return conn, nil
}