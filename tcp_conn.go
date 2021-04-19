package skeleton

import (
	"io"
	"net"
	"sync"

	"mlib.com/mlog"
)

type ConnSet map[net.Conn]struct{}

type TCPConn struct {
	sync.Mutex
	conn      net.Conn
	writeChan chan []byte
	closeFlag bool
}

func newTCPConn(conn net.Conn, pendingWriteNum int) *TCPConn {
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

	var bufMsgHeader []byte
	bufMsgHeader = make([]byte, p.GetHeaderLen())

	// read header
	n, err := io.ReadFull(tcpConn, bufMsgHeader)
	if err != nil {
		mlog.Errorf("read msg header failed:%v", err)
		return 0, nil, err
	}

	if uint32(n) < p.GetHeaderLen() {
		mlog.Errorf("msg too short")
		return 0, nil, NetErrMsgTooShort
	}

	payloadLen, cmd, err := p.ParseHeader(bufMsgHeader)
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

	// data
	payloadData := make([]byte, payloadLen)
	if _, err := io.ReadFull(tcpConn, payloadData); err != nil {
		mlog.Errorf("read payload data failed:%v", err)
		return 0, nil, err
	}

	return cmd, payloadData, nil
}
