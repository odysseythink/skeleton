package skeleton

import "time"

// one dispatcher per goroutine (goroutine not safe)
type Dispatcher struct {
	ChanTimer chan *Timer
}

func NewDispatcher(l int) *Dispatcher {
	disp := new(Dispatcher)
	disp.ChanTimer = make(chan *Timer, l)
	return disp
}

// Timer
type Timer struct {
	t    *time.Timer
	call *CallInfo
}

func (disp *Dispatcher) AfterFunc(d time.Duration, handler CallHandler, arg interface{}) *Timer {
	t := new(Timer)
	t.call = new(CallInfo)
	t.call.f = handler
	t.call.arg = arg
	t.t = time.AfterFunc(d, func() {
		disp.ChanTimer <- t
	})
	return t
}
