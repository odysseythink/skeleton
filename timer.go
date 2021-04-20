package skeleton

import (
	"fmt"
	"runtime"
	"time"

	"mlib.com/mlog"
)

const (
	timer_onetime = iota
	timer_period
)

// one dispatcher per goroutine (goroutine not safe)
type Dispatcher struct {
	ChanTimer chan *Timer
}

func NewDispatcher(l uint32) *Dispatcher {
	disp := new(Dispatcher)
	disp.ChanTimer = make(chan *Timer, l)
	return disp
}

func (disp *Dispatcher) AfterFunc(d time.Duration, handler CallHandler, arg interface{}) *Timer {
	t := new(Timer)
	t.call = new(CallInfo)
	t.call.f = handler
	t.call.args = make([]interface{}, 0)
	t.call.args = append(t.call.args, arg)
	t.timerType = timer_onetime
	t.t = time.AfterFunc(d, func() {
		disp.ChanTimer <- t
	})
	return t
}

func (disp *Dispatcher) PeriodFunc(d time.Duration, handler CallHandler, arg interface{}) *Timer {
	t := new(Timer)
	t.call = new(CallInfo)
	t.call.f = handler
	t.call.args = make([]interface{}, 0)
	t.call.args = append(t.call.args, arg)
	t.timerType = timer_period
	t.period = d
	t.t = time.AfterFunc(d, func() {
		disp.ChanTimer <- t
	})
	return t
}

// Timer
type Timer struct {
	t         *time.Timer
	timerType int
	period    time.Duration
	call      *CallInfo
}

func (t *Timer) exec(disp *Dispatcher) (err error) {
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
	if t.timerType == timer_period {
		t.t = time.AfterFunc(t.period, func() {
			disp.ChanTimer <- t
		})
	}
	switch t.call.f.(type) {
	case CallHandler:
		t.call.f.(CallHandler)(t.call.args[0])
	default:
		mlog.Warning("timer.go unkown callback type")
	}
	err = nil
	return
}
