package skeleton

import "reflect"

type Processor interface {
	// must goroutine safe
	Route(msg interface{}, userData interface{}) error
	// must goroutine safe
	Unmarshal(t reflect.Type, data []byte) (interface{}, error)
	// must goroutine safe
	Marshal(msg interface{}) ([][]byte, error)
	ParseHeader(header []byte) (uint32, uint32, error)
	GetHeaderLen() uint32
	GetMaxPayloadLen() uint32
	GetMinPayloadLen() uint32
	AssemblePacket(cmd uint32, payload interface{}) ([]byte, error)
}
