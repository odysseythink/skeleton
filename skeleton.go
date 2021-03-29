package skeleton

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type CallHandler func(interface{})

type CallInfo struct {
	f   CallHandler
	arg interface{}
}

type Skeleton struct {
	chanCall   chan *CallInfo
	wg         sync.WaitGroup
	closeSig   chan bool
	threadNum  uint32
	dispatcher *Dispatcher
}

func NewSkeleton(thread, maxHandler, timerDispatcherLen uint32) *Skeleton {
	return &Skeleton{
		chanCall:   make(chan *CallInfo, maxHandler),
		closeSig:   make(chan bool, thread),
		threadNum:  thread,
		dispatcher: NewDispatcher(timerDispatcherLen),
	}
}

func (s *Skeleton) Run() error {
	for i := 0; i < int(s.threadNum); i++ {
		go s.run(s.closeSig)
	}
	return nil
}

func (s *Skeleton) Destroy() {
	for i := 0; i < int(s.threadNum); i++ {
		s.closeSig <- true
	}
	s.wg.Wait()
}

func (s *Skeleton) AsyncExec(handler CallHandler, arg interface{}) {
	s.chanCall <- &CallInfo{
		f:   handler,
		arg: arg,
	}
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

	ci.f(ci.arg)
	err = nil
	return
}
