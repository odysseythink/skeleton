package skeleton

import "net"

type Agent interface {
	WriteMsg(msg interface{})
	Close()
	Destroy()
	UserData() interface{}
	SetUserData(data interface{})
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

type ConnAgent interface {
	Run()
	OnClose()
}
