package tftp

import (
	"net"
	"time"
)

func (s *Server) singlePortProcessRequests() error {
	shuttingDown := false
	for {
		select {
		case <-s.cancel.Done():
			shuttingDown = true
		default:
		}

		if shuttingDown {
			// So we not blocked forever waiting for a packet
			s.conn.SetReadDeadline(time.Now().Add(time.Second))
		}

		buf := make([]byte, s.maxBlockLen+4)
		cnt, localAddr, srcAddr, maxSz, err := s.getPacket(buf)
		if err != nil {
			if shuttingDown {
				s.wg.Wait()
				return nil
			}
			if s.hook != nil {
				s.hook.OnFailure(TransferStats{
					SenderAnticipateEnabled: s.sendAEnable,
				}, err)
			}
			continue
		}
		if cnt == 0 {
			continue
		}
		s.mu.Lock()
		if receiverChannel, ok := s.handlers[srcAddr.String()]; ok {
			// Packet received for a transfer in progress.
			s.mu.Unlock()
			select {
			case receiverChannel <- buf[:cnt]:
			default:
				// We don't want to block the main loop if a channel is full
			}
		} else {
			// No existing transfer for given source address. Start a new one.
			if shuttingDown {
				s.mu.Unlock()
				continue
			}
			lc := make(chan []byte, 1)
			s.handlers[srcAddr.String()] = lc
			s.mu.Unlock()
			go func() {
				err := s.handlePacket(localAddr, srcAddr, buf, cnt, maxSz, lc)
				if err != nil {
					if s.hook != nil {
						s.hook.OnFailure(TransferStats{
							SenderAnticipateEnabled: s.sendAEnable,
						}, err)
					}
					// Normally handlePacket starts an incoming or outgoing transfer,
					// and creates a connection object that cleans up the entry in handlers map
					// at the end of the transfer.
					// But when handlePacket fails, it doesn't create transfer and connection,
					// we still need to clean up the map entry.
					s.mu.Lock()
					delete(s.handlers, srcAddr.String())
					s.mu.Unlock()
				}
			}()
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
