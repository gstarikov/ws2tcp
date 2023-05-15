package relay

import (
	"github.com/gstarikov/ws2tcp/internal/api"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdd(t *testing.T) {
	tests := map[string]struct {
		chunks    []string
		respLines []api.ResponseLine
		err       bool
	}{
		"good": {
			chunks: []string{
				`{"RequestID":"4","Counter":2,"Payload":"0","CRC64":1}` + "\n",
			},
			respLines: []api.ResponseLine{{RequestID: "4", Counter: 2, Payload: "0", CRC64: 1}},
			err:       false,
		},
		"split once": {
			chunks: []string{
				`{"RequestID":"4","Counter":2,`,
				`"Payload":"0","CRC64":1}` + "\n",
			},
			respLines: []api.ResponseLine{{RequestID: "4", Counter: 2, Payload: "0", CRC64: 1}},
			err:       false,
		},
		"split two times": {
			chunks: []string{
				`{"RequestID":"4","Counter":2,`,
				`"Payload":"0","CRC64":1}` + "\n" + `{"RequestID":"5","Counter":3,`,
				`"Payload":"1","CRC64":2}` + "\n",
			},
			respLines: []api.ResponseLine{
				{RequestID: "4", Counter: 2, Payload: "0", CRC64: 1},
				{RequestID: "5", Counter: 3, Payload: "1", CRC64: 2},
			},
			err: false,
		}, "chunk no new line": {
			chunks: []string{
				`{"RequestID":"4","Counter":2,"Payload":"0","CRC64":1}`,
				"\n" + `{"RequestID":"5","Counter":3,"Payload":"1","CRC64":2}` + "\n",
				"",
			},
			respLines: []api.ResponseLine{
				{RequestID: "4", Counter: 2, Payload: "0", CRC64: 1},
				{RequestID: "5", Counter: 3, Payload: "1", CRC64: 2},
			},
			err: false,
		},
	}

	for testCase, testData := range tests {
		t.Run(testCase, func(t *testing.T) {
			acc := newAccumulator(60)
			res := make([]api.ResponseLine, 0, len(testData.respLines))
			for _, data := range testData.chunks {
				r, _, err := acc.add([]byte(data))
				if testData.err {
					require.Error(t, err)
				}
				if r != nil {
					res = append(res, *r)
				}
			}
			require.Equal(t, testData.respLines, res)
		})
	}
}
