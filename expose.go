package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"
import (
	"bytes"
	"unsafe"
)

const (
	ErrorEnd C.int64_t = iota
	ErrorContinuing
)

var hashBuffer [1024]byte
var statisticsBuffer [1024]byte

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

// GetError exposes the error byte buffer to the user that wants to read on it.
// If hasNext != 0 there is still more text to read in the error message.

//export GetError
func GetError(
	hasNext *C.int64_t, // [out]
) *C.char {
	var ptr *C.char
	var fits bool
	ptr, errorMsg, fits = writeToConstString(errorMsg)
	if fits {
		*hasNext = ErrorEnd
	} else {
		*hasNext = ErrorContinuing
	}
	return ptr
}

// -----------------------------------------------------------------------------
// HELPER FUNCTIONS
// -----------------------------------------------------------------------------

func writeToConstString(text string) (*C.char, string, bool) {
	var fits bool
	if len(text) > BufferSize {
		copy(generalTextBuffer[:], text[:BufferSize])
		generalTextBuffer[BufferSize] = 0
		text = text[BufferSize:]
		fits = false
	} else {
		fits = true
		n := copy(generalTextBuffer[:], text)
		generalTextBuffer[n] = 0
		text = ""
	}
	return (*C.char)(unsafe.Pointer(&generalTextBuffer[0])), text, fits
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
