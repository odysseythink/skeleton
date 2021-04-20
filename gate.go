package skeleton

import (
	"net"
	"reflect"
	"sync"
	"time"

	"mlib.com/mlog"
)

const (
	defaultKeepAlivePeriod = time.Second * 60
)

type Gate struct {
	MaxConnNum      uint32
	PendingWriteNum uint32
	MaxMsgLen       uint32
	Processor       Processor

	// tcp
	TCPAddr           string
	LenMsgLen         int
	LittleEndian      bool
	KeepAliveCheck    bool
	KeepAlivePeriod   time.Duration
	agentsIdleTime    map[*agent]time.Duration
	agentsIdleTimeMux sync.Mutex

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

type OnNewAgentCallback func(Agent)
type MsgHandler func(Agent, interface{})

func keepAlive(arg interface{}) {
	if _, ok := arg.(*Gate); ok {
		g := arg.(*Gate)
		g.agentsIdleTimeMux.Lock()
		for k := range g.agentsIdleTime {
			g.agentsIdleTime[k] += time.Second * 5
			if g.agentsIdleTime[k] >= g.KeepAlivePeriod {
				k.Close()
				delete(g.agentsIdleTime, k)
			}
		}
		g.agentsIdleTimeMux.Unlock()
	}
}

func (gate *Gate) Register(cmd uint32, msg interface{}, f MsgHandler) {
	if gate.messagesInfo == nil {
		gate.messagesInfo = make(map[uint32]*MsgInfo)
	}
	if gate.messagesCmd == nil {
		gate.messagesCmd = make(map[reflect.Type]uint32)
	}
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		mlog.Emerg("message pointer required")
	}
	if _, ok := gate.messagesInfo[cmd]; ok {
		mlog.Emergf("message cmd=0x%x is already registered", cmd)
	}
	if _, ok := gate.messagesCmd[msgType]; ok {
		mlog.Emergf("message %v is already registered", msgType.Name())
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
			if gate.KeepAliveCheck {
				if gate.agentsIdleTime == nil {
					gate.agentsIdleTime = make(map[*agent]time.Duration)
				}
				gate.agentsIdleTimeMux.Lock()
				gate.agentsIdleTime[a] = 0
				gate.agentsIdleTimeMux.Unlock()
			}

			return a
		}
		if gate.KeepAliveCheck && gate.ParentSkeleton != nil {
			if gate.KeepAlivePeriod == 0 {
				gate.KeepAlivePeriod = defaultKeepAlivePeriod
			}
			gate.ParentSkeleton.PeriodFunc(time.Second*5, keepAlive, gate)
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
		a.gate.agentsIdleTimeMux.Lock()
		a.gate.agentsIdleTime[a] = 0
		a.gate.agentsIdleTimeMux.Unlock()
		if data == nil && cmd == 0 {
			continue
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
	if a.gate.KeepAliveCheck && a.gate.agentsIdleTime != nil {
		a.gate.agentsIdleTimeMux.Lock()
		delete(a.gate.agentsIdleTime, a)
		a.gate.agentsIdleTimeMux.Unlock()
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

func (a *agent) LocalAddr() net.Addr {
	return a.LocalAddr()
}

func (a *agent) RemoteAddr() net.Addr {
	return a.RemoteAddr()
}
