tftp
====

TFTP server and client library for Golang

	import "github.com/pin/tftp"

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

Client provides TFTP client functionality. It requires remote address and
optional logger

Uploading file to server example

	addr, e := net.ResolveUDPAddr("udp", "example.org:69")
	if e != nil {
		...
	}
	file, e := os.Open("/etc/passwd")
	if e != nil {
		...
	}
	r := bufio.NewReader(file)
	log := log.New(os.Stderr, "", log.Ldate | log.Ltime)
	c := tftp.Client{addr, log}
	c.Put(filename, mode, func(writer *io.PipeWriter) {
		_, writeError := r.WriteTo(writer)
		if writeError != nil {
			fmt.Fprintf(os.Stderr, "Can't put %s: %v\n", filename, writeError);
		} else {
			fmt.Fprintf(os.Stderr, "Put %s (%d bytes)\n", filename, n);
		}
		writer.Close()
	})
	
Downloading file from server example

	addr, e := net.ResolveUDPAddr("udp", "example.org:69")
	if e != nil {
		...
	}
	file, e := os.Create("/var/tmp/debian.img")
	if e != nil {
		...
	}
	w := bufio.NewWriter(file)
	log := log.New(os.Stderr, "", log.Ldate | log.Ltime)
	c := tftp.Client{addr, log}
	c.Get(filename, mode, func(reader *io.PipeReader) {
		_, readError := w.ReadFrom(reader)
		if readError != nil {
			fmt.Fprintf(os.Stderr, "Can't get %s: %v\n", filename, readError);
		} else {
			fmt.Fprintf(os.Stderr, "Got %s (%d bytes)\n", filename, n);
		}
		w.Flush()
		file.Close()
	})
