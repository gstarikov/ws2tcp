package api

import (
	"encoding/binary"
	"hash/crc64"
)

type Request struct {
	RequestID string
}
type ResponseLine struct {
	RequestID string //will b ethe same as in initial request
	Counter   uint64 //sequential counter for outgoing messages
	Payload   string //random string
	CRC64     uint64 //crc64(requestID+counter+payload)
}

// GetCRC64 only for testing, because tcp already contains crc32
func (resp ResponseLine) GetCRC64() uint64 {
	table := crc64.MakeTable(crc64.ECMA) // there is singleton inside
	crcSum := crc64.New(table)

	_, _ = crcSum.Write([]byte(resp.RequestID))

	counterByte := binary.LittleEndian.AppendUint64(nil, resp.Counter)
	_, _ = crcSum.Write(counterByte)

	_, _ = crcSum.Write([]byte(resp.Payload))

	sum64 := crcSum.Sum64()

	return sum64
}
