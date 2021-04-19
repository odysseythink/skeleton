package skeleton

import (
	"fmt"
	"sync"
	"testing"
)

func TestAsyncExec(t *testing.T) {
	ske := &Skeleton{
		ThreadNum:          20,
		MaxHandler:         1024,
		MaxTimerDispatcher: 1024,
	}
	ske.Run()
	var wg sync.WaitGroup
	for index := 0; index < 100; index++ {
		wg.Add(2)
		ske.AsyncExec(func(arg interface{}) {

			switch arg.(type) {
			case int:
				t.Log("int arg:", arg.(int))
			case uint:
				t.Log("uint arg:", arg.(uint))
			case uint32:
				t.Log("uint32 arg:", arg.(uint32))
			case string:
				t.Log("string arg:", arg.(string))
			}
			wg.Done()
		}, index)
		ske.AsyncExec(func(arg interface{}) {
			switch arg.(type) {
			case int:
				t.Log("int arg:", arg.(int))
			case uint:
				t.Log("uint arg:", arg.(uint))
			case uint32:
				t.Log("uint32 arg:", arg.(uint32))
			case string:
				t.Log("string arg:", arg.(string))
			}
			wg.Done()
		}, fmt.Sprintf("index=%v", index))
	}
	wg.Wait()
}
