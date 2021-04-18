package skeleton

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"time"
)

type CallHandler func(interface{})

type CallInfo struct {
	f        interface{}
	args     []interface{}
	callType int // 0 - normal call; 1 -- network message call ; 2 -- connection callback function
	argType  reflect.Type
}

type Skeleton struct {
	ThreadNum          uint32
	MaxHandler         uint32
	MaxTimerDispatcher uint32
	MyGate             *Gate
	chanCall           chan *CallInfo
	wg                 sync.WaitGroup
	closeSig           chan bool
	dispatcher         *Dispatcher
}

// func NewSkeleton(thread, maxHandler, timerDispatcherLen uint32) *Skeleton {
// 	return &Skeleton{
// 		chanCall:   make(chan *CallInfo, maxHandler),
// 		closeSig:   make(chan bool, thread),
// 		threadNum:  thread,
// 		dispatcher: NewDispatcher(timerDispatcherLen),
// 	}
// }

const (
	defaultMaxHandler         = 1024
	defaultThreadNum          = 32
	defaultMaxTimerDispatcher = 1024
)

func (s *Skeleton) init() {
	if s.MaxHandler == 0 {
		s.MaxHandler = defaultMaxHandler
	}
	if s.ThreadNum == 0 {
		s.ThreadNum = defaultThreadNum
	}
	if s.MaxTimerDispatcher == 0 {
		s.MaxTimerDispatcher = defaultMaxTimerDispatcher
	}
	s.chanCall = make(chan *CallInfo, s.MaxHandler)

	s.dispatcher = NewDispatcher(s.MaxTimerDispatcher)
	if s.MyGate != nil {
		s.closeSig = make(chan bool, s.ThreadNum+1)
		s.MyGate.ParentSkeleton = s
	} else {
		s.closeSig = make(chan bool, s.ThreadNum)
	}
}

func (s *Skeleton) Run() error {
	s.init()
	for i := 0; i < int(s.ThreadNum); i++ {
		go s.run(s.closeSig)
	}
	if s.MyGate != nil {
		go func() {
			s.wg.Add(1)
			s.MyGate.Run(s.closeSig)
			s.wg.Done()
		}()
	}
	return nil
}

func (s *Skeleton) Destroy() {
	var threadNum uint32
	if s.MyGate != nil {
		threadNum = s.ThreadNum + 1
	} else {
		threadNum = s.ThreadNum
	}
	for i := 0; i < int(threadNum); i++ {
		s.closeSig <- true
	}
	s.wg.Wait()
}

func (s *Skeleton) AsyncExec(handler, arg interface{}) {
	fmt.Println("......", "skeleton.go AsyncExec 1:")
	c := &CallInfo{
		f:        handler,
		callType: 0,
		args:     make([]interface{}, 0),
	}
	c.args = append(c.args, arg)
	fmt.Println("args len=", len(c.args))
	s.chanCall <- c
}

func (s *Skeleton) AsyncNetworkMsgExec(handler func(Agent, interface{}), agent Agent, arg interface{}) {
	fmt.Println("......", "skeleton.go AsyncNetworkMsgExec 1:")
	c := &CallInfo{
		f:        handler,
		callType: 1,
		args:     make([]interface{}, 0),
	}
	c.args = append(c.args, agent)
	c.args = append(c.args, arg)
	s.chanCall <- c
}

func (s *Skeleton) AsyncNetworkCallbackExec(handler func(interface{}), agent interface{}) {
	fmt.Println("......", "skeleton.go AsyncNetworkCallbackExec 1:")

	c := &CallInfo{
		f:        handler,
		callType: 2,
		args:     make([]interface{}, 0),
	}
	c.args = append(c.args, agent)
	s.chanCall <- c
}

func (s *Skeleton) AfterFunc(d time.Duration, handler CallHandler, arg interface{}) *Timer {
	return s.dispatcher.AfterFunc(d, handler, arg)
}

func (s *Skeleton) PeriodFunc(d time.Duration, handler CallHandler, arg interface{}) *Timer {
	return s.dispatcher.PeriodFunc(d, handler, arg)
}

func (s *Skeleton) run(closeSig chan bool) {
	s.wg.Add(1)
	defer func() {
		s.wg.Done()
	}()
	for {
		select {
		case <-closeSig:
			return
		case call, ok := <-s.chanCall:
			if !ok {
				return
			}
			fmt.Println("......", "skeleton.go run 1:")
			s.exec(call)
		case t := <-s.dispatcher.ChanTimer:
			t.exec(s.dispatcher)
		}
	}
}

func (s *Skeleton) exec(ci *CallInfo) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if LenStackBuf > 0 {
				buf := make([]byte, LenStackBuf)
				l := runtime.Stack(buf, false)
				err = fmt.Errorf("%v: %s", r, buf[:l])
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	fmt.Println("......", "skeleton.go exec 1:")
	// execute
	switch ci.f.(type) {
	case func(interface{}):
		fmt.Println("......", "skeleton.go exec 2:")
		ci.f.(func(interface{}))(ci.args[0])
	case func(interface{}, interface{}):
		fmt.Println("......", "skeleton.go exec 3:")
		ci.f.(func(interface{}, interface{}))(ci.args[0], ci.args[1])
	case func(Agent, interface{}):
		fmt.Println("......", "skeleton.go exec 4:")
		ci.f.(func(Agent, interface{}))(ci.args[0].(Agent), ci.args[1])
	}
	err = nil
	return
}
