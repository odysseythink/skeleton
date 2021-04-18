package skeleton

import (
	"errors"
	"net"
)

var (
	NetErrNoProcessor = errors.New("message don't have processor")
	NetErrMsgTooShort = errors.New("msg is too short")
	NetErrMsgTooLong  = errors.New("msg is too long")
)

type Conn interface {
	ReadMsg(p Processor) (uint32, []byte, error)
	Write(pkg []byte)
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Close()
	Destroy()
}
