package skeleton

type Agent interface {
	WriteMsg(msg interface{})
	Close()
	Destroy()
	UserData() interface{}
	SetUserData(data interface{})
}

type ConnAgent interface {
	Run()
	OnClose()
}
