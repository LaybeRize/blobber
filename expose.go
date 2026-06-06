package main

/*
#include <stdlib.h>
#include <stdint.h>

// just a typedef — no header needed, pure C type syntax
typedef void (*WriteCallback)(const char*);
typedef const char* (*ReadCallback)();

static void write_callback(WriteCallback cb, const char* s) {
    cb(s);
}

static const char* read_callback(ReadCallback cb) {
	return cb();
}
*/
import "C"
import (
	"bytes"
	"unsafe"
)

const (
	BufferSize        = 1024 * 1024
	BufferTruncation  = 3
	BufferAssumedSize = 1 << 28
)

var hashBuffer [1024]byte
var statisticsBuffer [1024]byte
var streamingValues *[]string
var generalTextBuffer [BufferSize + BufferTruncation + 1]byte

// -----------------------------------------------------------------------------
// DIRECT CALL FUNCTIONS
// -----------------------------------------------------------------------------

// BlobOpen opens a file handle for reading or writing.
// Exactly one of readFile / writeFile must be a non-empty string.
// If opening for write the compression string is not allowed to be null pointer (but is allowed to be an empty string)

//export BlobOpen
func BlobOpen(
	readFile *C.char, // [in]
	writeFile *C.char, // [in]
	compression *C.int64_t, // [in]
) C.int64_t {
	return C.int64_t(
		BlobOpenGo(
			C.GoString(readFile),
			C.GoString(writeFile),
			(*int64)(unsafe.Pointer(compression)),
		))
}

// BlobCompress streams a file into the open blob with optional change detection.
//
// there is one way the call terminates early with rcOK. If the path is not a file the system sets file_changed=-1 and
// returns with success.
//
// file_hash points to the pointer of a buffer.
//    if the pointer is NULL     -> first version, always write
//    if the pointer has a value -> compare length/mtime/hash, skip if identical
//
// On write: fills file_length, file_position, file_last_modified_ns,
//           file_hash, file_changed=1.
// On skip:  sets file_changed=0, all other pointers unchanged.

//export BlobCompress
func BlobCompress(
	filePath *C.char, // [in]
	fileLength *C.uint64_t, // [in/out]
	filePosition *C.uint64_t, // [in/out]
	fileLastModifiedNs *C.uint64_t, // [in/out]
	fileHash **C.char, // [in/out] If pre-allocated at least 65-byte
	fileChanged *C.int64_t, // [out] 0=false 1=true
) C.int64_t {
	hashRead := readDoublePointer(fileHash)
	rtCode, newHash := BlobCompressGo(C.GoString(filePath),
		(*uint64)(unsafe.Pointer(fileLength)),
		(*uint64)(unsafe.Pointer(filePosition)),
		(*uint64)(unsafe.Pointer(fileLastModifiedNs)),
		hashRead,
		(*int64)(unsafe.Pointer(fileChanged)),
	)
	writeDoublePointer(fileHash, &hashBuffer[0], newHash)
	return C.int64_t(rtCode)
}

// BlobDecompress streams the specified amount of bytes into the opened file at the path given.
// If given position is != currentFilePos first reads to position before
// reading and writing file_length bytes to the file.

//export BlobDecompress
func BlobDecompress(
	targetPath *C.char, // [in]
	position *C.uint64_t, // [in/out]
	fileLength C.uint64_t, // [in]
) C.int64_t {
	return C.int64_t(
		BlobDecompressGo(
			string(C.GoString(targetPath)),
			(*uint64)(unsafe.Pointer(position)),
			uint64(fileLength)))
}

// BlobClose flushes and closes the file handle.

//export BlobClose
func BlobClose() C.int64_t {
	return C.int64_t(BlobCloseGo())
}

//export BlobCloseWithStatistics
func BlobCloseWithStatistics(
	compressionRate **C.char, // [in/out] If pre-allocated at least 10-byte
) C.int64_t {
	retCode, statistics := BlobCloseWithStatisticsGo()
	writeDoublePointer(compressionRate, &statisticsBuffer[0], statistics)
	return C.int64_t(retCode)
}

// -----------------------------------------------------------------------------
// MANAGER FUNCTIONS
// -----------------------------------------------------------------------------

//export LoadOverview
func LoadOverview(
	path *C.char, // [in]
) C.int64_t {
	retCode := LoadOverviewGo(C.GoString(path))
	if retCode == rcOK {
		streamingValues = &currentOverview.RepositoryNames
	}
	return C.int64_t(retCode)
}

//export CloseOverview
func CloseOverview() C.int64_t {
	return C.int64_t(CloseOverviewGo())
}

//export StreamArrayToPython
func StreamArrayToPython(
	callback C.WriteCallback, // [in]
) {
	if streamingValues == nil {
		return
	}
	for _, s := range *streamingValues {
		cStr := C.CString(s)
		C.write_callback(callback, cStr)
		C.free(unsafe.Pointer(cStr))
	}
}

//export StreamArrayFromPython
func StreamArrayFromPython(
	callback C.ReadCallback, // [in]
) {
	stream := make([]string, 0)
	for {
		cStr := C.read_callback(callback)
		if cStr == nil {
			break
		}
		stream = append(stream, C.GoString(cStr))
	}
	streamingValues = &stream
}

// -----------------------------------------------------------------------------
// HELPER FUNCTIONS
// -----------------------------------------------------------------------------

// GetError exposes the error byte buffer to the user that wants to read on it.
// If the error message would be longer then 1MiB it gets truncated with ...\x00 at the 1 MiB border

//export GetError
func GetError() *C.char {
	var n int
	if len(errorMsg) > BufferSize {
		n = copy(generalTextBuffer[:], errorMsg[:BufferSize])
		for range BufferTruncation {
			generalTextBuffer[n] = '.'
			n++
		}
	} else {
		n = copy(generalTextBuffer[:], errorMsg)
	}
	generalTextBuffer[n] = 0
	errorMsg = ""
	return (*C.char)(unsafe.Pointer(&generalTextBuffer[0]))
}

func readDoublePointer(ptr **C.char) *string {
	if *ptr == nil {
		return nil
	}
	//noinspection ALL
	buf := unsafe.Slice((*byte)(unsafe.Pointer(*ptr)), BufferAssumedSize)
	n := bytes.IndexByte(buf[:], 0)
	val := string(buf[:n])
	return &val
}

func writeDoublePointer(ptr **C.char, alternativeBuffer *byte, value string) {
	if *ptr == nil {
		*ptr = (*C.char)(unsafe.Pointer(alternativeBuffer))
	}
	//noinspection ALL
	buf := unsafe.Slice((*byte)(unsafe.Pointer(*ptr)), BufferAssumedSize)
	n := copy(buf[:], value)
	buf[n] = 0
}

func main() {}
