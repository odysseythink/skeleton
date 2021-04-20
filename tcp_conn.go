package skeleton

import (
	"fmt"
	"io"
	"net"
	"sync"

	"mlib.com/mlog"
)

type ConnSet map[net.Conn]struct{}

type TCPConn struct {
	sync.Mutex
	conn         net.Conn
	writeChan    chan []byte
	closeFlag    bool
	rb           *RingBuffer
	tmpPkgBuffer []byte
}

func newTCPConn(conn net.Conn, pendingWriteNum uint32) *TCPConn {
	tcpConn := new(TCPConn)
	tcpConn.conn = conn
	tcpConn.writeChan = make(chan []byte, pendingWriteNum)

	go func() {
		for b := range tcpConn.writeChan {
			if b == nil {
				break
			}

			_, err := conn.Write(b)
			if err != nil {
				break
			}
		}

		conn.Close()
		tcpConn.Lock()
		tcpConn.closeFlag = true
		tcpConn.Unlock()
	}()

	return tcpConn
}

func (tcpConn *TCPConn) doDestroy() {
	tcpConn.conn.(*net.TCPConn).SetLinger(0)
	tcpConn.conn.Close()

	if !tcpConn.closeFlag {
		close(tcpConn.writeChan)
		tcpConn.closeFlag = true
	}
}

func (tcpConn *TCPConn) Destroy() {
	tcpConn.Lock()
	defer tcpConn.Unlock()

	tcpConn.doDestroy()
}

func (tcpConn *TCPConn) Close() {
	tcpConn.Lock()
	defer tcpConn.Unlock()
	if tcpConn.closeFlag {
		return
	}

	tcpConn.doWrite(nil)
	tcpConn.closeFlag = true
}

func (tcpConn *TCPConn) doWrite(b []byte) {
	if len(tcpConn.writeChan) == cap(tcpConn.writeChan) {
		mlog.Infof("close conn: channel full")
		tcpConn.doDestroy()
		return
	}

	tcpConn.writeChan <- b
}

// b must not be modified by the others goroutines
func (tcpConn *TCPConn) Write(b []byte) {
	tcpConn.Lock()
	defer tcpConn.Unlock()
	if tcpConn.closeFlag || b == nil {
		return
	}

	tcpConn.doWrite(b)
}

func (tcpConn *TCPConn) Read(b []byte) (int, error) {
	return tcpConn.conn.Read(b)
}

func (tcpConn *TCPConn) LocalAddr() net.Addr {
	return tcpConn.conn.LocalAddr()
}

func (tcpConn *TCPConn) RemoteAddr() net.Addr {
	return tcpConn.conn.RemoteAddr()
}

func (tcpConn *TCPConn) ReadMsg(p Processor) (uint32, []byte, error) {
	if p == nil {
		mlog.Error("process cant be nil")
		return 0, nil, NetErrNoProcessor
	}
	if tcpConn.rb == nil {
		tcpConn.rb = NewRingBuffer(int(p.GetMaxPayloadLen() + p.GetHeaderLen()))
	}
	if tcpConn.tmpPkgBuffer == nil {
		tcpConn.tmpPkgBuffer = make([]byte, p.GetHeaderLen()+p.GetMaxPayloadLen())
	}

	if tcpConn.rb.Free() == tcpConn.rb.Capacity() { // 本地缓存中没有数据
		// read header
		n, err := io.ReadFull(tcpConn, tcpConn.tmpPkgBuffer[:p.GetHeaderLen()])
		if err != nil {
			mlog.Errorf("read msg header failed:%v", err)
			return 0, nil, err
		}

		if uint32(n) < p.GetHeaderLen() { // 取出的包太短,存在分包现象,存入本地缓存
			mlog.Warning("msg too short, cache it")
			if tcpConn.rb.Free() >= n {
				tcpConn.rb.Write(tcpConn.tmpPkgBuffer[:n])
				return 0, nil, nil // 返回的err如果为nil,但是数据也为nil,代表没有取出数据
			}
			return 0, nil, NetErrNoSpaceToCache
		}

		cmd, payloadData, err := tcpConn.parseHeaderAndReadPayload(p, tcpConn.tmpPkgBuffer[:p.GetHeaderLen()])
		if err != nil {
			mlog.Errorf("parse msg header failed:%v", err)
			return 0, nil, err
		}
		return cmd, payloadData, nil
	} else { // 本地缓存中有数据,优先先处理本地缓存中的数据
		cachedPkgLen := tcpConn.rb.Capacity() - tcpConn.rb.Free()
		if cachedPkgLen < int(p.GetHeaderLen()) { // 缓存的数据不够一个包头长度,先收齐包头,再首paylaod
			// read header
			n, err := io.ReadFull(tcpConn, tcpConn.tmpPkgBuffer[cachedPkgLen:p.GetHeaderLen()])
			if err != nil {
				mlog.Errorf("read msg header failed:%v", err)
				return 0, nil, err
			}
			if n < (int(p.GetHeaderLen()) - cachedPkgLen) { // 依然继续缓存
				if tcpConn.rb.Free() >= n {
					_, err = tcpConn.rb.Write(tcpConn.tmpPkgBuffer[cachedPkgLen : cachedPkgLen+n])
					if err != nil {
						return 0, nil, fmt.Errorf("cache pkg faild:%v", err)
					}
					return 0, nil, nil // 返回的err如果为nil,但是数据也为nil,代表没有取出数据
				}
				return 0, nil, NetErrNoSpaceToCache
			}
			// 读满一个包头了,把ringbuffer里面的数据读出来,凑齐一个包头,开始处理
			r1, err := tcpConn.rb.Read(tcpConn.tmpPkgBuffer[:cachedPkgLen])
			if err != nil {
				return 0, nil, fmt.Errorf("read cache pkg faild:%v", err)
			}
			if r1 < cachedPkgLen {
				return 0, nil, fmt.Errorf("read invalid cache pkg lenght")
			}
			cmd, payloadData, err := tcpConn.parseHeaderAndReadPayload(p, tcpConn.tmpPkgBuffer[:p.GetHeaderLen()])
			if err != nil {
				mlog.Errorf("parse msg header failed:%v", err)
				return 0, nil, err
			}
			return cmd, payloadData, nil
		} else { // 缓存的数据包括包头
			r1, err := tcpConn.rb.Read(tcpConn.tmpPkgBuffer[:cachedPkgLen])
			if err != nil {
				return 0, nil, fmt.Errorf("read cache pkg faild:%v", err)
			}
			if r1 < cachedPkgLen {
				return 0, nil, fmt.Errorf("read invalid cache pkg lenght")
			}
			payloadLen, cmd, err := p.ParseHeader(tcpConn.tmpPkgBuffer[:p.GetHeaderLen()])
			if err != nil {
				mlog.Errorf("parse msg header failed:%v", err)
				return 0, nil, err
			}
			needReadLen := payloadLen - (uint32(cachedPkgLen) - p.GetHeaderLen())
			// read payload
			r2, err := io.ReadFull(tcpConn, tcpConn.tmpPkgBuffer[cachedPkgLen:cachedPkgLen+int(needReadLen)])
			if err != nil {
				mlog.Errorf("read msg header failed:%v", err)
				return 0, nil, err
			}
			if r2 < int(needReadLen) { // 还是没有凑齐一个包头,缓存,下次再读
				if (r2 + cachedPkgLen) <= tcpConn.rb.Free() {
					if _, err := tcpConn.rb.Write(tcpConn.tmpPkgBuffer[:cachedPkgLen+r2]); err != nil {
						return 0, nil, fmt.Errorf("cache pkg faild:%v", err)
					}
					return 0, nil, nil
				}
				return 0, nil, NetErrNoSpaceToCache
			}

			return cmd, tcpConn.tmpPkgBuffer[p.GetHeaderLen() : p.GetHeaderLen()+payloadLen], nil
		}
	}
}

func (tcpConn *TCPConn) parseHeaderAndReadPayload(p Processor, header []byte) (uint32, []byte, error) {
	if len(header) != int(p.GetHeaderLen()) {
		return 0, nil, fmt.Errorf("invalid header lenght")
	}

	payloadLen, cmd, err := p.ParseHeader(header)
	if err != nil {
		mlog.Errorf("parse msg header failed:%v", err)
		return 0, nil, err
	}

	// check len
	if payloadLen > p.GetMaxPayloadLen() {
		mlog.Errorf("parse payloadLen from header is too long")
		return 0, nil, NetErrMsgTooLong
	} else if payloadLen < p.GetMinPayloadLen() {
		mlog.Errorf("parse payloadLen from header is too short")
		return 0, nil, NetErrMsgTooShort
	}
	if payloadLen == 0 { // 有cmd,但是没有payload,用于keepalive这种特殊的包
		return cmd, nil, nil
	}
	// data
	n, err := io.ReadFull(tcpConn, tcpConn.tmpPkgBuffer[p.GetHeaderLen():p.GetHeaderLen()+payloadLen])
	if err != nil {
		mlog.Errorf("read payload data failed:%v", err)
		return 0, nil, err
	}
	if n < int(payloadLen) { // 存在分包现象,这个包没有取完,缓存数据,留待下次处理
		if (n + int(p.GetHeaderLen())) < tcpConn.rb.Free() {
			_, err := tcpConn.rb.Write(header)
			if err != nil {
				return 0, nil, fmt.Errorf("cache pkg faild:%v", err)
			}
			_, err = tcpConn.rb.Write(tcpConn.tmpPkgBuffer[p.GetHeaderLen() : p.GetHeaderLen()+uint32(n)])
			if err != nil {
				return 0, nil, fmt.Errorf("cache pkg faild:%v", err)
			}
			return 0, nil, nil // 返回的err如果为nil,但是数据也为nil,代表没有取出数据
		}
		return 0, nil, NetErrNoSpaceToCache
	}
	return cmd, tcpConn.tmpPkgBuffer[p.GetHeaderLen() : p.GetHeaderLen()+payloadLen], nil
}
