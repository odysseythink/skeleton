package json

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/name5566/leaf/chanrpc"
	"github.com/name5566/leaf/log"
)

type Processor struct {
	msgInfo       map[string]*MsgInfo
	msgHeaderLen  uint32
	littleEndian  bool
	maxPayloadLen uint32
	minPayloadLen uint32
}

type MsgInfo struct {
	msgType       reflect.Type
	msgRouter     *chanrpc.Server
	msgHandler    MsgHandler
	msgRawHandler MsgHandler
}

type MsgHandler func([]interface{})

type MsgRaw struct {
	msgID      string
	msgRawData json.RawMessage
}

func NewProcessor() *Processor {
	p := new(Processor)
	p.msgInfo = make(map[string]*MsgInfo)
	p.msgHeaderLen = 16
	p.littleEndian = false
	p.minPayloadLen = 0
	p.maxPayloadLen = 1024
	return p
}

// It's dangerous to call the method on routing or marshaling (unmarshaling)
func (p *Processor) Register(msg interface{}) string {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	if msgID == "" {
		log.Fatal("unnamed json message")
	}
	if _, ok := p.msgInfo[msgID]; ok {
		log.Fatal("message %v is already registered", msgID)
	}

	i := new(MsgInfo)
	i.msgType = msgType
	p.msgInfo[msgID] = i
	return msgID
}

// It's dangerous to call the method on routing or marshaling (unmarshaling)
func (p *Processor) SetRouter(msg interface{}, msgRouter *chanrpc.Server) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %v not registered", msgID)
	}

	i.msgRouter = msgRouter
}

// It's dangerous to call the method on routing or marshaling (unmarshaling)
func (p *Processor) SetHandler(msg interface{}, msgHandler MsgHandler) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %v not registered", msgID)
	}

	i.msgHandler = msgHandler
}

// It's dangerous to call the method on routing or marshaling (unmarshaling)
func (p *Processor) SetRawHandler(msgID string, msgRawHandler MsgHandler) {
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %v not registered", msgID)
	}

	i.msgRawHandler = msgRawHandler
}

// goroutine safe
func (p *Processor) Route(msg interface{}, userData interface{}) error {
	// raw
	if msgRaw, ok := msg.(MsgRaw); ok {
		i, ok := p.msgInfo[msgRaw.msgID]
		if !ok {
			return fmt.Errorf("message %v not registered", msgRaw.msgID)
		}
		if i.msgRawHandler != nil {
			i.msgRawHandler([]interface{}{msgRaw.msgID, msgRaw.msgRawData, userData})
		}
		return nil
	}

	// json
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		return errors.New("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		return fmt.Errorf("message %v not registered", msgID)
	}
	if i.msgHandler != nil {
		i.msgHandler([]interface{}{msg, userData})
	}
	if i.msgRouter != nil {
		i.msgRouter.Go(msgType, msg, userData)
	}
	return nil
}

// goroutine safe
func (p *Processor) Unmarshal(t reflect.Type, data []byte) (interface{}, error) {
	msg := reflect.New(t.Elem()).Interface()
	return msg, json.Unmarshal(data, msg)
}

// goroutine safe
func (p *Processor) Marshal(msg interface{}) ([][]byte, error) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		return nil, errors.New("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	if _, ok := p.msgInfo[msgID]; !ok {
		return nil, fmt.Errorf("message %v not registered", msgID)
	}

	// data
	m := map[string]interface{}{msgID: msg}
	data, err := json.Marshal(m)
	return [][]byte{data}, err
}

func (p *Processor) ParseHeader(header []byte) (uint32, uint32, error) {
	if len(header) != int(p.msgHeaderLen) {
		return 0, 0, fmt.Errorf("invalid message header len,want=%v, real=%v", p.msgHeaderLen, len(header))
	}

	// parse cmd
	var cmd uint32
	if p.littleEndian {
		cmd = uint32(binary.LittleEndian.Uint32(header[:4]))
	} else {
		cmd = uint32(binary.BigEndian.Uint32(header[:4]))
	}

	// parse payload lenght
	var payloadLen uint32
	if p.littleEndian {
		payloadLen = uint32(binary.LittleEndian.Uint32(header[4:8]))
	} else {
		payloadLen = uint32(binary.BigEndian.Uint32(header[4:8]))
	}

	return payloadLen - p.msgHeaderLen, cmd, nil
}

func (p *Processor) GetHeaderLen() uint32 {
	return p.msgHeaderLen
}

func (p *Processor) GetMaxPayloadLen() uint32 {
	return p.maxPayloadLen
}

func (p *Processor) GetMinPayloadLen() uint32 {
	return p.minPayloadLen
}

func (p *Processor) AssemblePacket(cmd uint32, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal payload failed:%v", err)
	}
	pkgBuff := make([]byte, 16+len(data))
	// assemble cmd
	if p.littleEndian {
		binary.LittleEndian.PutUint32(pkgBuff[:4], cmd)
	} else {
		binary.BigEndian.PutUint32(pkgBuff[:4], cmd)
	}

	// assemble lenght
	if p.littleEndian {
		binary.LittleEndian.PutUint32(pkgBuff[4:8], uint32(16+len(data)))
	} else {
		binary.BigEndian.PutUint32(pkgBuff[4:8], uint32(16+len(data)))
	}
	copy(pkgBuff[16:], data)
	return pkgBuff, nil
}
