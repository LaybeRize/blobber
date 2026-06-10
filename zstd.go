package main

/*
#pragma GCC diagnostic ignored "-Wbuiltin-macro-redefined"
#define ZSTD_STATIC_LINKING_ONLY
#include "zstd.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"io"
	"unsafe"
)

// -----------------------------------------------------------------------------
// Writer
// -----------------------------------------------------------------------------

// ZStdWriter compresses data written to it and forwards the compressed bytes
// to an underlying io.Writer. Call Close() to flush and finalise the zstd frame.
//
// All buffers are C-allocated so that no Go pointer ever crosses the CGo
// boundary inside a struct field, satisfying the CGo pointer rules.
type ZStdWriter struct {
	dst       io.Writer
	cstream   *C.ZSTD_CStream
	inBufSize C.size_t
	input     C.ZSTD_inBuffer
	output    C.ZSTD_outBuffer
}

// NewZStdWriter creates a new ZStdWriter that compresses into dst at the given
// level. Pass -1 to use the zstd default level (currently 6).
// The caller must call Close() when done.
func NewZStdWriter(dst io.Writer, level int) (*ZStdWriter, error) {
	if level == -1 {
		level = 6
	}

	stream := C.ZSTD_createCStream()
	if stream == nil {
		return nil, fmt.Errorf("zstd: ZSTD_createCStream returned NULL")
	}

	rc := C.ZSTD_initCStream(stream, C.int(level))
	if C.ZSTD_isError(rc) != 0 {
		C.ZSTD_freeCStream(stream)
		return nil, fmt.Errorf("zstd: ZSTD_initCStream: %s", C.GoString(C.ZSTD_getErrorName(rc)))
	}

	inSize := C.ZSTD_CStreamInSize()
	outSize := C.ZSTD_CStreamOutSize()

	return &ZStdWriter{
		dst:       dst,
		cstream:   stream,
		inBufSize: inSize,
		input: C.ZSTD_inBuffer{
			src:  C.malloc(inSize),
			size: 0,
			pos:  0,
		},
		output: C.ZSTD_outBuffer{
			dst:  C.malloc(outSize),
			size: outSize,
			pos:  0,
		},
	}, nil
}

// Write compresses p and writes the compressed output to the underlying writer.
// Implements io.Writer. May write zero compressed bytes for small inputs due to
// zstd's internal buffering - all output is guaranteed to be flushed on Close.
func (w *ZStdWriter) Write(p []byte) (int, error) {
	total := 0

	for len(p) > 0 {
		// Chunk p into inBufSize pieces so we never overflow the C buffer.
		chunk := len(p)
		if C.size_t(chunk) > w.inBufSize {
			chunk = int(w.inBufSize)
		}

		// Copy chunk into C memory - avoids Go pointer inside ZSTD_inBuffer.
		C.memcpy(w.input.src, unsafe.Pointer(&p[0]), C.size_t(chunk))

		w.input.size = C.size_t(chunk)
		w.input.pos = 0

		// Drive the stream until the entire input chunk is consumed.
		for w.input.pos < w.input.size {
			w.output.pos = 0

			rc := C.ZSTD_compressStream(w.cstream, &w.output, &w.input)
			if C.ZSTD_isError(rc) != 0 {
				return total, fmt.Errorf("zstd: ZSTD_compressStream: %s",
					C.GoString(C.ZSTD_getErrorName(rc)))
			}

			if w.output.pos > 0 {
				if _, err := w.dst.Write(unsafe.Slice((*byte)(w.output.dst), w.output.pos)); err != nil {
					return total, err
				}
			}
		}

		total += chunk
		p = p[chunk:]
	}
	return total, nil
}

// Flush flushes zstd's internal buffers to the underlying writer without
// closing the zstd frame. After Flush the receiver can still accept more
// Write calls. This is useful if the underlying writer is a network connection
// and the other side needs to decompress incrementally.
func (w *ZStdWriter) Flush() error {
	for {
		w.output.pos = 0

		remaining := C.ZSTD_flushStream(w.cstream, &w.output)
		if C.ZSTD_isError(remaining) != 0 {
			return fmt.Errorf("zstd: ZSTD_flushStream: %s",
				C.GoString(C.ZSTD_getErrorName(remaining)))
		}

		if w.output.pos > 0 {
			if _, err := w.dst.Write(unsafe.Slice((*byte)(w.output.dst), w.output.pos)); err != nil {
				return err
			}
		}

		if remaining == 0 {
			return nil
		}
		// remaining > 0: output buffer was full, loop to drain more.
	}
}

// Close finalises the zstd frame (writes the end-of-frame marker) and frees
// the C-allocated buffers and stream. The underlying writer is NOT closed.
// Close must be called exactly once; further calls to Write or Close are
// undefined.
func (w *ZStdWriter) Close() error {
	defer func() {
		C.ZSTD_freeCStream(w.cstream)
		C.free(unsafe.Pointer(w.input.src))
		C.free(unsafe.Pointer(w.output.dst))
		w.cstream = nil
		w.input.src = nil
		w.output.dst = nil
	}()

	// ZSTD_endStream writes the frame epilogue. It may need multiple calls if
	// the output buffer fills up before the epilogue is complete.
	for {
		w.output.pos = 0

		remaining := C.ZSTD_endStream(w.cstream, &w.output)
		if C.ZSTD_isError(remaining) != 0 {
			return fmt.Errorf("zstd: ZSTD_endStream: %s",
				C.GoString(C.ZSTD_getErrorName(remaining)))
		}

		if w.output.pos > 0 {
			if _, err := w.dst.Write(unsafe.Slice((*byte)(w.output.dst), w.output.pos)); err != nil {
				return err
			}
		}

		if remaining == 0 {
			return nil
		}
	}
}

// -----------------------------------------------------------------------------
// Reader
// -----------------------------------------------------------------------------

// ZStdReader decompresses data read from an underlying io.Reader on demand.
// It implements io.Reader and io.Closer.
//
// The tricky part of decompression is that one call to ZSTD_decompressStream
// can produce more output than the caller asked for, or less output than the
// output buffer could hold, depending on block boundaries. We therefore
// maintain a pending slice that holds decompressed bytes not yet returned to
// the caller.
type ZStdReader struct {
	src       io.Reader
	dstream   *C.ZSTD_DStream
	inBufSize C.size_t
	input     C.ZSTD_inBuffer
	output    C.ZSTD_outBuffer

	// pending holds decompressed bytes that zstd produced but the caller
	// hasn't consumed yet. It is a sub-slice of outBuf's memory, re-wrapped
	// as a Go slice for convenient indexing - safe because we never pass it
	// back to C.
	pending []byte

	srcEOF bool // true once src returns io.EOF
}

// NewZStdReader creates a ZStdReader that decompresses from src.
// The caller must call Close() to free C resources when done.
func NewZStdReader(src io.Reader) (*ZStdReader, error) {
	stream := C.ZSTD_createDStream()
	if stream == nil {
		return nil, fmt.Errorf("zstd: ZSTD_createDStream returned NULL")
	}

	rc := C.ZSTD_initDStream(stream)
	if C.ZSTD_isError(rc) != 0 {
		C.ZSTD_freeDStream(stream)
		return nil, fmt.Errorf("zstd: ZSTD_initDStream: %s", C.GoString(C.ZSTD_getErrorName(rc)))
	}

	inSize := C.ZSTD_DStreamInSize()
	outSize := C.ZSTD_DStreamOutSize()

	r := &ZStdReader{
		src:       src,
		dstream:   stream,
		inBufSize: inSize,
		input: C.ZSTD_inBuffer{
			src:  C.malloc(inSize),
			size: 0,
			pos:  0,
		},
		output: C.ZSTD_outBuffer{
			dst:  C.malloc(outSize),
			size: outSize,
			pos:  0,
		},
	}
	return r, nil
}

// Read decompresses into p and returns the number of bytes written.
// Implements io.Reader. Returns io.EOF when the zstd frame is complete and
// the underlying reader is exhausted.
func (r *ZStdReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	written := 0

	for written < len(p) {
		// 1. Drain whatever is already decompressed and pending.
		if len(r.pending) > 0 {
			n := copy(p[written:], r.pending)
			r.pending = r.pending[n:]
			written += n
			continue
		}

		// 2. No pending output. Check if we've already signalled EOF.
		if r.srcEOF && r.input.pos >= r.input.size {
			if written > 0 {
				return written, nil
			}
			return 0, io.EOF
		}

		// 3. If zstd has consumed everything in inBuf, refill from src.
		if r.input.pos >= r.input.size {
			n, err := r.src.Read(unsafe.Slice((*byte)(r.input.src), r.inBufSize))
			if n > 0 {
				r.input.size = C.size_t(n)
				r.input.pos = 0
			}
			if err == io.EOF {
				r.srcEOF = true
				if n == 0 {
					// Nothing new and src is done - return what we have.
					if written > 0 {
						return written, nil
					}
					return 0, io.EOF
				}
			} else if err != nil {
				return written, err
			}
		}

		// 4. Run ZSTD_decompressStream to produce output into outBuf.
		r.output.pos = 0

		rc := C.ZSTD_decompressStream(r.dstream, &r.output, &r.input)
		if C.ZSTD_isError(rc) != 0 {
			return written, fmt.Errorf("zstd: ZSTD_decompressStream: %s",
				C.GoString(C.ZSTD_getErrorName(rc)))
		}

		if r.output.pos > 0 {
			// Wrap outBuf as a Go slice for easy copying - we never pass this
			// slice to C, so it is safe despite pointing at C memory.
			r.pending = unsafe.Slice((*byte)(r.output.dst), r.output.pos)
		}

		// rc == 0 means the frame is complete. If src is also exhausted we
		// will hit the EOF path on the next iteration after draining pending.
	}

	return written, nil
}

// Close frees the C-allocated stream and buffers. The underlying reader is
// NOT closed. Must be called exactly once.
func (r *ZStdReader) Close() error {
	C.ZSTD_freeDStream(r.dstream)
	C.free(unsafe.Pointer(r.input.src))
	C.free(unsafe.Pointer(r.output.dst))
	r.dstream = nil
	r.input.src = nil
	r.output.dst = nil
	return nil
}
