package stdcopy

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func frame(t byte, payload string) []byte {
	h := make([]byte, 8)
	h[0] = t
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, []byte(payload)...)
}

func TestDemux_SplitsStdoutStderr(t *testing.T) {
	var src bytes.Buffer
	src.Write(frame(1, "out1"))
	src.Write(frame(2, "err1"))
	src.Write(frame(1, "out2"))

	var out, errb bytes.Buffer
	n, err := Demux(&out, &errb, &src)
	require.NoError(t, err)
	require.Equal(t, int64(12), n)
	require.Equal(t, "out1out2", out.String())
	require.Equal(t, "err1", errb.String())
}

func TestDemux_HandlesPayloadSpanningReads(t *testing.T) {
	// a frame whose payload is larger than the internal buffer chunk
	big := bytes.Repeat([]byte("x"), 70000)
	var src bytes.Buffer
	src.Write(frame(1, string(big)))
	var out, errb bytes.Buffer
	_, err := Demux(&out, &errb, &src)
	require.NoError(t, err)
	require.Equal(t, 70000, out.Len())
}
