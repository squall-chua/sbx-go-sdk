// Package stdcopy demultiplexes Docker's multiplexed stream format used by the
// sandboxd exec/attach endpoint: repeating [type, 0,0,0, size_be32][payload]
// frames where type 1 = stdout, 2 = stderr. This mirrors moby's stdcopy without
// pulling in the moby/moby dependency.
package stdcopy

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	stdin  = 0
	stdout = 1
	stderr = 2
	hdrLen = 8
)

// Demux reads the multiplexed stream from src, writing stdout frames to outW and
// stderr frames to errW. It returns the total number of payload bytes written and
// the first non-EOF error encountered. It returns nil error on clean EOF.
func Demux(outW, errW io.Writer, src io.Reader) (int64, error) {
	var written int64
	hdr := make([]byte, hdrLen)
	buf := make([]byte, 32*1024)
	for {
		if _, err := io.ReadFull(src, hdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return written, nil
			}
			return written, err
		}
		var dst io.Writer
		switch hdr[0] {
		case stdin, stdout:
			dst = outW
		case stderr:
			dst = errW
		default:
			return written, errors.New("stdcopy: unknown stream type")
		}
		size := int64(binary.BigEndian.Uint32(hdr[4:8]))
		for size > 0 {
			chunk := int64(len(buf))
			if size < chunk {
				chunk = size
			}
			n, err := src.Read(buf[:chunk])
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return written, werr
				}
				written += int64(n)
				size -= int64(n)
			}
			if err != nil {
				if err == io.EOF && size > 0 {
					return written, io.ErrUnexpectedEOF
				}
				if err == io.EOF {
					return written, nil
				}
				return written, err
			}
		}
	}
}
