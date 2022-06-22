package tftp

import (
	"net"
)

func (s *Server) singlePortProcessRequests() error {
	var (
		localAddr  net.IP
		cnt, maxSz int
		srcAddr    net.Addr
		err        error
		buf        []byte
	)
	defer func() {
		if r := recover(); r != nil {
			// We've received a new connection on the same IP+Port tuple
			// as a previous connection before garbage collection has occured
			s.handlers[srcAddr.String()] = make(chan []byte, 1)
			go func(localAddr net.IP, remoteAddr *net.UDPAddr, buffer []byte, n, maxBlockLen int, listener chan []byte) {
				err := s.handlePacket(localAddr, remoteAddr, buffer, n, maxBlockLen, listener)
				if err != nil && s.hook != nil {
					s.hook.OnFailure(TransferStats{
						SenderAnticipateEnabled: s.sendAEnable,
					}, err)
				}

			}(localAddr, srcAddr.(*net.UDPAddr), buf, cnt, maxSz, s.handlers[srcAddr.String()])
			s.singlePortProcessRequests()
		}
	}()
	for {
		select {
		case q := <-s.quit:
			q <- struct{}{}
			return nil
		case handlersToFree := <-s.runGC:
			for _, handler := range handlersToFree {
				delete(s.handlers, handler)
			}
		default:
			buf = s.bufPool.Get().([]byte)
			cnt, localAddr, srcAddr, maxSz, err = s.getPacket(buf)
			if err != nil || cnt == 0 {
				if s.hook != nil {
					s.hook.OnFailure(TransferStats{
						SenderAnticipateEnabled: s.sendAEnable,
					}, err)
				}
				s.bufPool.Put(buf)
				continue
			}
			if receiverChannel, ok := s.handlers[srcAddr.String()]; ok {
				select {
				case receiverChannel <- buf[:cnt]:
				default:
					// We don't want to block the main loop if a channel is full
				}
			} else {
				s.handlers[srcAddr.String()] = make(chan []byte, 1)
				go func(localAddr net.IP, remoteAddr *net.UDPAddr, buffer []byte, n, maxBlockLen int, listener chan []byte) {
					err := s.handlePacket(localAddr, remoteAddr, buffer, n, maxBlockLen, listener)
					if err != nil && s.hook != nil {
						s.hook.OnFailure(TransferStats{
							SenderAnticipateEnabled: s.sendAEnable,
						}, err)
					}

				}(localAddr, srcAddr.(*net.UDPAddr), buf, cnt, maxSz, s.handlers[srcAddr.String()])
			}
		}
	}
}

func (s *Server) getPacket(buf []byte) (int, net.IP, *net.UDPAddr, int, error) {
	if s.conn6 != nil {
		cnt, control, srcAddr, err := s.conn6.ReadFrom(buf)
		if err != nil || cnt == 0 {
			return 0, nil, nil, 0, err
		}
		var localAddr net.IP
		maxSz := blockLength
		if control != nil {
			localAddr = control.Dst
			if intf, err := net.InterfaceByIndex(control.IfIndex); err == nil {
				// mtu - ipv4 overhead - udp overhead
				maxSz = intf.MTU - 28
			}
		}
		return cnt, localAddr, srcAddr.(*net.UDPAddr), maxSz, nil
	} else if s.conn4 != nil {
		cnt, control, srcAddr, err := s.conn4.ReadFrom(buf)
		if err != nil || cnt == 0 {
			return 0, nil, nil, 0, err
		}
		var localAddr net.IP
		maxSz := blockLength
		if control != nil {
			localAddr = control.Dst
			if intf, err := net.InterfaceByIndex(control.IfIndex); err == nil {
				// mtu - ipv6 overhead - udp overhead
				maxSz = intf.MTU - 48
			}
		}
		return cnt, localAddr, srcAddr.(*net.UDPAddr), maxSz, nil
	} else {
		cnt, srcAddr, err := s.conn.ReadFrom(buf)
		if err != nil {
			return 0, nil, nil, 0, err
		}
		return cnt, nil, srcAddr.(*net.UDPAddr), blockLength, nil
	}
}

// internalGC collects all the finished signals from each connection's goroutine
// The main loop is sent the key to be nil'ed after the gcInterval has passed
func (s *Server) internalGC() {
	var completedHandlers []string
	for {
		select {
		case newHandler := <-s.gcCollect:
			completedHandlers = append(completedHandlers, newHandler)
			if len(completedHandlers) > s.gcThreshold {
				s.runGC <- completedHandlers
				completedHandlers = nil
			}
		}
	}
}
