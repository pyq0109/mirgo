package main

import "github.com/pyq0109/mirgo/internal/protocol"

type NpcObject struct {
	*BaseObject
	Appr   uint16
	Script string
}

func NewNpcObject(name string, id int32, appr uint16) *NpcObject {
	base := NewBaseObject(name, id)
	return &NpcObject{
		BaseObject: base,
		Appr:       appr,
	}
}

func (o *NpcObject) Feature() int32 {
	return protocol.MakeMonsterFeature(10, 0, o.Appr)
}
