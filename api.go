package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const BufferSize = 1024 * 1024
const ChunkSize = 4 * 1024 * 1024
const BufferAssumedSize = 1 << 28
const rcOK int64 = 1
const rcErr int64 = 0
const (
	SkippedFile int64 = iota - 1
	FileUnchanged
	FileChanged
)

var errorMsg = ""
var generalTextBuffer [BufferSize + 1]byte

var currentReadFile *DecompressionWriter
var currentWriteFile *CompressionWriter

func BlobOpenGo(readFile string, writeFile string, compression *int64) int64 {
	if currentWriteFile != nil || currentReadFile != nil {
		return setErr("BlobOpen: called this function while a file is still open.")
	}

	readSet := readFile != ""
	writeSet := writeFile != ""

	if readSet == writeSet {
		if readSet {
			return setErr("BlobOpen: both read_file and write_file provided")
		}
		return setErr("BlobOpen: both read_file and write_file are empty")
	}

	var compLevel = -1
	var err error

	if compression != nil {
		compLevel = int(*compression)
	}

	if writeSet {
		err, currentWriteFile = openFileWrite(writeFile, compLevel)
	} else {
		err, currentReadFile = openFileRead(readFile)
	}

	if err != nil {
		return setErr(fmt.Sprintf("BlobOpen: fcbopen failed: %v", err))
	}

	return rcOK
}

func BlobCompressGo(
	filePath string,
	fileLength *uint64,
	filePosition *uint64,
	fileLastModifiedNs *uint64,
	prevHash *string,
	fileChanged *int64,
) (int64, string) {
	if currentWriteFile == nil {
		return setErr("BlobCompress: can't write to a file that isn't opened."), ""
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return setErr(fmt.Sprintf("BlobCompress: stat %q: %v", filePath, err)), ""
	}

	if !info.Mode().IsRegular() {
		*fileChanged = SkippedFile
		return rcOK, ""
	}

	curLen := uint64(info.Size())
	curMtime := uint64(info.ModTime().UnixNano())

	isFirst := prevHash == nil
	var newHash string
	var written uint64

	if !isFirst {
		// Cheap check: length + mtime first
		if curLen == *fileLength && curMtime == *fileLastModifiedNs {
			*fileChanged = FileUnchanged
			return rcOK, ""
		}
		var herr error
		newHash, herr = hashFile(filePath)
		if herr != nil {
			return setErr(fmt.Sprintf("BlobCompress: hash %q: %v", filePath, herr)), ""
		}
		if newHash == *prevHash {
			*fileChanged = FileUnchanged
			return rcOK, ""
		}
	}

	err, written, newHash = currentWriteFile.compressFileWithHash(filePath)
	if err != nil {
		return setErr(fmt.Sprintf("BlobCompress: write %q: %v", filePath, err)), ""
	}

	*fileLength = written
	*filePosition = currentWriteFile.position
	*fileLastModifiedNs = curMtime
	*fileChanged = FileChanged

	currentWriteFile.position += written
	return rcOK, newHash
}

func BlobDecompressGo(
	targetPath string,
	position *uint64,
	fileLength uint64,
) int64 {
	if currentReadFile == nil {
		return setErr("BlobDecompress: can't read from a file that isn't opened.")
	}
	err := currentReadFile.decompressFile(targetPath, position, fileLength)
	if err != nil {
		return setErr(fmt.Sprintf("BlobDecompress: %v", err))
	}
	return rcOK
}

func BlobCloseGo() int64 {
	if currentWriteFile == nil && currentReadFile == nil {
		return setErr("BlobClose: can't close a file that isn't open.")
	}
	var err error
	if currentWriteFile != nil {
		err = currentWriteFile.close()
	} else {
		err = currentReadFile.close()
	}
	if err != nil {
		return setErr(fmt.Sprintf("BlobClose: close failed:\n%v", err))
	}
	currentReadFile = nil
	currentWriteFile = nil
	return rcOK
}

func BlobCloseWithStatisticsGo() (int64, string) {
	var bytesProcessed uint64 = 0
	blobFile := ""
	if currentWriteFile != nil {
		bytesProcessed = currentWriteFile.position
		blobFile = currentWriteFile.filePath
	}
	retCode := BlobCloseGo()
	if retCode != rcOK {
		return retCode, "???"
	}
	stats, err := os.Stat(blobFile)
	if err != nil {
		return rcOK, "???"
	}
	precent := (float64(stats.Size()) / float64(bytesProcessed)) * 100
	return rcOK, fmt.Sprintf("%.3f%%", precent)
}

// -----------------------------------------------------------------------------
// HELPER FUNCTIONS
// -----------------------------------------------------------------------------

func setErr(errMsg string) int64 {
	errorMsg = errMsg
	return rcErr
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, ChunkSize)

	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
