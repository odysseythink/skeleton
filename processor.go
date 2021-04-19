package skeleton

import "reflect"

type Processor interface {
	// must goroutine safe
	Unmarshal(t reflect.Type, data []byte) (interface{}, error)
	// must goroutine safe
	Marshal(cmd uint32, msg interface{}) ([]byte, error)
	ParseHeader(header []byte) (uint32, uint32, error)
	GetHeaderLen() uint32
	GetMaxPayloadLen() uint32
	GetMinPayloadLen() uint32
}
