package skeleton

import (
	"reflect"

	"mlib.com/mlog"
)

type Gate struct {
	MaxConnNum      int
	PendingWriteNum int
	MaxMsgLen       uint32
	Processor       Processor

	// tcp
	TCPAddr        string
	LenMsgLen      int
	LittleEndian   bool
	messagesInfo   map[uint32]*MsgInfo
	messagesCmd    map[reflect.Type]uint32
	OnNewAgentCb   OnNewAgentCallback
	OnCloseAgentCb OnNewAgentCallback
	ParentSkeleton *Skeleton
}

type MsgInfo struct {
	msgType       reflect.Type
	msgHandler    MsgHandler
	msgRawHandler MsgHandler
}

type OnNewAgentCallback func(interface{})
type MsgHandler func(Agent, interface{})

func (gate *Gate) Register(cmd uint32, msg interface{}, f MsgHandler) {
	if gate.messagesInfo == nil {
		gate.messagesInfo = make(map[uint32]*MsgInfo)
	}
	if gate.messagesCmd == nil {
		gate.messagesCmd = make(map[reflect.Type]uint32)
	}
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		mlog.Fatal("message pointer required")
	}
	if _, ok := gate.messagesInfo[cmd]; ok {
		mlog.Fatal("message cmd=0x%x is already registered", cmd)
	}
	if _, ok := gate.messagesCmd[msgType]; ok {
		mlog.Fatal("message %v is already registered", msgType.Name())
	}

	i := new(MsgInfo)
	i.msgType = msgType
	i.msgHandler = f
	gate.messagesInfo[cmd] = i
	gate.messagesCmd[msgType] = cmd
}

func (gate *Gate) Run(closeSig chan bool) {
	var tcpServer *TCPServer
	if gate.TCPAddr != "" {
		tcpServer = new(TCPServer)
		tcpServer.Addr = gate.TCPAddr
		tcpServer.MaxConnNum = gate.MaxConnNum
		tcpServer.PendingWriteNum = gate.PendingWriteNum
		tcpServer.LenMsgLen = gate.LenMsgLen
		tcpServer.MaxMsgLen = gate.MaxMsgLen
		tcpServer.LittleEndian = gate.LittleEndian
		tcpServer.NewAgent = func(conn *TCPConn) ConnAgent {
			a := &agent{conn: conn, gate: gate}
			if gate.OnNewAgentCb != nil && gate.ParentSkeleton != nil {
				gate.ParentSkeleton.asyncNetworkCallbackExec(gate.OnNewAgentCb, a)
			}
			return a
		}
	}

	if tcpServer != nil {
		tcpServer.Start()
	}

	<-closeSig

	if tcpServer != nil {
		tcpServer.Close()
	}
}

func (gate *Gate) OnDestroy() {}

type agent struct {
	conn     Conn
	gate     *Gate
	userData interface{}
}

func (a *agent) Run() {
	for {
		cmd, data, err := a.conn.ReadMsg(a.gate.Processor)
		if err != nil {
			mlog.Infof("read message: %v", err)
			break
		}

		info, ok := a.gate.messagesInfo[cmd]
		if !ok {
			mlog.Errorf("invalid message cmd=0x%x", cmd)
		} else {
			msg, err := a.gate.Processor.Unmarshal(info.msgType, data)
			if err != nil {
				mlog.Infof("unmarshal message error: %v", err)
				break
			}
			a.gate.ParentSkeleton.asyncNetworkMsgExec(info.msgHandler, a, msg)
		}
	}
}

func (a *agent) OnClose() {
	if a.gate.OnCloseAgentCb != nil && a.gate.ParentSkeleton != nil {
		a.gate.ParentSkeleton.asyncNetworkCallbackExec(a.gate.OnCloseAgentCb, a)
	}
}

func (a *agent) WriteMsg(msg interface{}) {
	if a.gate.Processor != nil {
		msgType := reflect.TypeOf(msg)
		if msgType == nil || msgType.Kind() != reflect.Ptr {
			mlog.Errorf("message pointer required")
			return
		}
		cmd, ok := a.gate.messagesCmd[msgType]
		if !ok {
			mlog.Errorf("message %v not register", msgType.Name())
			return
		}
		pkg, err := a.gate.Processor.Marshal(cmd, msg)
		if err != nil {
			mlog.Errorf("marshal message %v error: %v", reflect.TypeOf(msg), err)
			return
		}
		a.conn.Write(pkg)
	}
}

func (a *agent) Close() {
	a.conn.Close()
}

func (a *agent) Destroy() {
	a.conn.Destroy()
}

func (a *agent) UserData() interface{} {
	return a.userData
}

func (a *agent) SetUserData(data interface{}) {
	a.userData = data
}
