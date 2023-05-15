package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gstarikov/ws2tcp/internal/api"
)

type accumulator struct {
	buf     []byte
	prevBuf []byte
}

func newAccumulator(bufSize int) *accumulator {
	return &accumulator{
		buf: make([]byte, 0, bufSize),
	}
}

// add returns:
// - object (in case of success)
// - read more flag
// - error in case of unrecoverable error
func (a *accumulator) add(newChunk []byte) (*api.ResponseLine, bool, error) {
	// join with prev buf
	a.prevBuf = append([]byte{}, a.buf...)
	a.buf = append(a.buf, newChunk...)
	//search for new line character
	nLineIdx := bytes.IndexByte(a.buf, '\n')
	if nLineIdx == -1 {
		//read until we will find new line mark
		return nil, true, nil
	}

	bufSize := len(a.buf)    // store filled size
	a.buf = a.buf[:nLineIdx] // virtually truncate

	respLine := &api.ResponseLine{}
	err := json.Unmarshal(a.buf, respLine)
	if err != nil {
		fmt.Printf("buf-> %s\norigBuf -> %s\n chunk -> %s\n prevBuf-> %s\n",
			a.buf, a.buf[:bufSize], newChunk, a.prevBuf)
	}

	// copy tail to begin
	tailLen := bufSize - nLineIdx - 1
	if tailLen == 0 { //no tail
		a.buf = a.buf[:0]
	} else {
		a.buf = a.buf[:bufSize] // restore original size
		copy(a.buf, a.buf[nLineIdx+1:bufSize])
		a.buf = a.buf[:tailLen] //truncate bach
	}
	return respLine, false, err
}
