//go:build !cli

package main

/*
#include <stdlib.h>
#include <stdint.h>

// just a typedef — no header needed, pure C type syntax
typedef void (*WriteCallback)(const char*);
typedef const char* (*ReadCallback)();
typedef void (*StatCallback)(int64_t, uint64_t);

static void write_callback(WriteCallback cb, const char* s) {
    cb(s);
}

static const char* read_callback(ReadCallback cb) {
	return cb();
}

static void stat_callback(StatCallback cb, int64_t f, uint64_t b) {
	cb(f, b);
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
	compressionRate **C.char, // [out] If pre-allocated at least 10-byte
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

//export RegisterNewRepository
func RegisterNewRepository(
	repositoryName *C.char, // [in]
) C.int64_t {
	retCode := RegisterNewRepositoryGo(C.GoString(repositoryName))
	if retCode == rcOK {
		streamingValues = &currentRepo.VersionNames
	}
	return C.int64_t(retCode)
}

//export LoadRepository
func LoadRepository(
	repositoryName *C.char, // [in]
) C.int64_t {
	retCode := LoadRepositoryGo(C.GoString(repositoryName))
	if retCode == rcOK {
		streamingValues = &currentRepo.VersionNames
	}
	return C.int64_t(retCode)
}

//export RegisterNewVersion
func RegisterNewVersion(
	versionName *C.char, // [in]
) C.int64_t {
	return C.int64_t(RegisterNewVersionGo(C.GoString(versionName)))
}

//export LoadVersion
func LoadVersion(
	versionName *C.char, // [in]
) C.int64_t {
	retCode := LoadVersionGo(C.GoString(versionName))
	if retCode == rcOK {
		tempArray := make([]string, len(currentVersion.Files))
		for i, file := range currentVersion.Files {
			tempArray[i] = file.FilePath
		}
		streamingValues = &tempArray
	}
	return C.int64_t(retCode)
}

//export LoadAndSetPreviousVersion
func LoadAndSetPreviousVersion(
	previousVersionName *C.char, // [in]
) C.int64_t {
	return C.int64_t(LoadAndSetPreviousVersionGo(C.GoString(previousVersionName)))
}

//export WriteToVersion
func WriteToVersion(
	compression *C.int64_t, // [in]
	callback C.ReadCallback, // [in]
	bytesProcessed *C.uint64_t, // [out]
	compressionRate **C.char, // [out] If pre-allocated at least 10-byte
) C.int64_t {
	retCode := StartWriteToVersionGo((*int64)(unsafe.Pointer(compression)))
	if retCode != rcOK {
		return C.int64_t(retCode)
	}
	*bytesProcessed = 0
	for {
		cStr := C.read_callback(callback)
		if cStr == nil {
			break
		}
		retCode, _ = TryWritingToVersionGo(C.GoString(cStr), (*uint64)(unsafe.Pointer(bytesProcessed)))
		if retCode != rcOK {
			return C.int64_t(retCode)
		}
	}
	retCode, statistics := StopWriteToVersionGo()
	writeDoublePointer(compressionRate, &statisticsBuffer[0], statistics)
	return C.int64_t(retCode)
}

// Important Notice: Before this function is called, StreamArrayToDLL() must be called with the
// list of file patterns to Read, even if the list is empty, so that the internally kept array that is used
// in the function is cleaned up. If StreamArrayToDLL() is not called before, the function might not behave
// as expected.

//export ReadFromVersion
func ReadFromVersion(
	overwriteExistingFiles C.int64_t, // [in]
	callback C.StatCallback, // [in]
) C.int64_t {
	internalCallback := func(filesWritten int64, bytesWritten uint64) {
		C.stat_callback(callback, C.int64_t(filesWritten), C.uint64_t(bytesWritten))
	}
	return C.int64_t(ReadFromVersionGo(*streamingValues, overwriteExistingFiles != 0, &internalCallback))
}

// Important Notice: Before this function is called, StreamArrayToDLL() must be called with the
// list of file patterns to Read, even if the list is empty, so that the internally kept array that is used
// in the function is cleaned up. If StreamArrayToDLL() is not called before, the function might not behave
// as expected. The result then overwrites the array and can be read via StreamArrayFromDLL().

//export EstimateRead
func EstimateRead(
	overwriteExistingFiles C.int64_t, // [in]
) C.int64_t {
	retCode, result := EstimateReadGo(*streamingValues, overwriteExistingFiles != 0)
	if retCode == rcOK {
		streamingValues = &result
	}
	return C.int64_t(retCode)
}

// -----------------------------------------------------------------------------
// HELPER FUNCTIONS
// -----------------------------------------------------------------------------

//export UpdateParameter
func UpdateParameter(
	fileDivider C.int64_t, // [in]
	totalByteMarker C.uint64_t, // [in]
) {
	Divider = int64(fileDivider)
	ByteMarker = uint64(totalByteMarker)
}

//export StreamArrayFromDLL
func StreamArrayFromDLL(
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

//export StreamArrayToDLL
func StreamArrayToDLL(
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
