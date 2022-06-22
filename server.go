package tftp

import (
	"fmt"
	"io"
	"log"
	"net"
)

/*
Server provides TFTP server functionality. It requires bind address, handlers
for read and write requests and optional logger.

	func HandleWrite(filename string, r *io.PipeReader) {
		buffer := &bytes.Buffer{}
		c, e := buffer.ReadFrom(r)
		if e != nil {
			fmt.Fprintf(os.Stderr, "Can't receive %s: %v\n", filename, e)
		} else {
			fmt.Fprintf(os.Stderr, "Received %s (%d bytes)\n", filename, c)
			...
		}
	}
	func HandleRead(filename string, w *io.PipeWriter) {
		if fileExists {
			...
			c, e := buffer.WriteTo(w)
			if e != nil {
				fmt.Fprintf(os.Stderr, "Can't send %s: %v\n", filename, e)
			} else {
				fmt.Fprintf(os.Stderr, "Sent %s (%d bytes)\n", filename, c)
			}
			w.Close()
		} else {
			w.CloseWithError(fmt.Errorf("File not exists: %s", filename))
		}
	}
	...
	addr, e := net.ResolveUDPAddr("udp", ":69")
	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
	log := log.New(os.Stderr, "TFTP", log.Ldate | log.Ltime)
	s := tftp.Server{addr, HandleWrite, HandleRead, log}
	e = s.Serve()
	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
*/
type Server struct {
	BindAddr     *net.UDPAddr
	ReadHandler  func(filename string, r *io.PipeReader)
	WriteHandler func(filename string, w *io.PipeWriter)
	Log          *log.Logger
}

func (s Server) Serve() error {
	conn, e := net.ListenUDP("udp", s.BindAddr)
	if e != nil {
		return e
	}
	for {
		e = s.processRequest(conn)
		if e != nil {
			if s.Log != nil {
				s.Log.Printf("%v\n", e)
			}
		}
	}
}

func (s Server) processRequest(conn *net.UDPConn) error {
	var buffer []byte
	buffer = make([]byte, MAX_DATAGRAM_SIZE)
	n, remoteAddr, e := conn.ReadFromUDP(buffer)
	if e != nil {
		return fmt.Errorf("Failed to read data from client: %v", e)
	}
	p, e := ParsePacket(buffer[:n])
	if e != nil {
		return nil
	}
	switch p := p.(type) {
	case *WRQ:
		s.Log.Printf("got WRQ (filename=%s, mode=%s)", p.Filename, p.Mode)
		trasnmissionConn, e := s.transmissionConn()
		if e != nil {
			return fmt.Errorf("Could not start transmission: %v", e)
		}
		reader, writer := io.Pipe()
		r := &receiver{remoteAddr, trasnmissionConn, writer, p.Filename, p.Mode, s.Log}
		go s.ReadHandler(p.Filename, reader)
		// Writing zero bytes to the pipe just to check for any handler errors early
		var null_buffer []byte
		null_buffer = make([]byte, 0)
		_, e = writer.Write(null_buffer)
		if e != nil {
			errorPacket := ERROR{1, e.Error()}
			trasnmissionConn.WriteToUDP(errorPacket.Pack(), remoteAddr)
			s.Log.Printf("sent ERROR (code=%d): %s", 1, e.Error())
			return e
		}
		go r.run(true)
	case *RRQ:
		s.Log.Printf("got RRQ (filename=%s, mode=%s)", p.Filename, p.Mode)
		trasnmissionConn, e := s.transmissionConn()
		if e != nil {
			return fmt.Errorf("Could not start transmission: %v", e)
		}
		reader, writer := io.Pipe()
		r := &sender{remoteAddr, trasnmissionConn, reader, p.Filename, p.Mode, s.Log}
		go s.WriteHandler(p.Filename, writer)
		go r.run(true)
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
