package tftp

import (
	"net"
)

func (s *Server) singlePortProcessRequests() error {
	for {
		select {
		case <-s.cancel.Done():
			s.wg.Wait()
			return nil
		default:
			buf := make([]byte, s.maxBlockLen+4)
			cnt, localAddr, srcAddr, maxSz, err := s.getPacket(buf)
			if err != nil || cnt == 0 {
				if s.hook != nil {
					s.hook.OnFailure(TransferStats{
						SenderAnticipateEnabled: s.sendAEnable,
					}, err)
				}
				continue
			}
			s.Lock()
			if receiverChannel, ok := s.handlers[srcAddr.String()]; ok {
				s.Unlock()
				select {
				case receiverChannel <- buf[:cnt]:
				default:
					// We don't want to block the main loop if a channel is full
				}
			} else {
				lc := make(chan []byte, 1)
				s.handlers[srcAddr.String()] = lc
				s.Unlock()
				go func() {
					err := s.handlePacket(localAddr, srcAddr, buf, cnt, maxSz, lc)
					if err != nil && s.hook != nil {
						s.hook.OnFailure(TransferStats{
							SenderAnticipateEnabled: s.sendAEnable,
						}, err)
					}

				}()
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
