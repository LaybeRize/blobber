package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"
)

const (
	ChunkSize = 4 * 1024 * 1024

	rcOK  int64 = 1
	rcErr int64 = 0

	SkippedFile int64 = iota - 1
	FileUnchanged
	FileChanged
)

var errorMsg = ""

// -----------------------------------------------------------------------------
// DIRECT CALL FUNCTIONS
// -----------------------------------------------------------------------------

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
// MANAGER FUNCTIONS
// -----------------------------------------------------------------------------

var currentOverviewFolder string
var currentOverview *RepositoryOverview
var currentRepo *RepositoryManifest
var currentVersion *VersionManifest
var previousVersion *VersionManifest
var previousFilesMap map[string]*FileManifest

const (
	OverviewName     = "general.overview"
	RepositorySuffix = ".repo"
	VersionSuffix    = ".version"
	BlobSuffix       = ".blob"
)

func LoadOverviewGo(overviewFolder string) int64 {
	overviewPath := path.Join(overviewFolder, OverviewName)
	currentOverview = &RepositoryOverview{
		RepositoryNames: make([]string, 0),
		RepositoryPaths: make([]string, 0),
	}
	if _, err := os.Stat(overviewPath); errors.Is(err, os.ErrNotExist) {
		currentOverviewFolder = overviewFolder
		return rcOK
	}
	err := currentOverview.StreamFromFile(overviewPath)
	if err != nil {
		currentOverview = nil
		return setErr(fmt.Sprintf("LoadOverview: %v", err))
	}
	currentOverviewFolder = overviewFolder
	return rcOK
}

func CloseOverviewGo() int64 {
	if currentOverview == nil {
		return setErr("CloseOverview: can't close an overview that is not open.")
	}
	if err := os.MkdirAll(currentOverviewFolder, 0755); err != nil {
		return setErr(fmt.Sprintf("CloseOverview: failed to create file directory '%s' : %v",
			currentOverviewFolder, err))
	}
	err := currentOverview.StreamToFile(path.Join(currentOverviewFolder, OverviewName))
	if err != nil {
		return setErr(fmt.Sprintf("CloseOverview: %v", err))
	}
	err = closeRepo()
	if err != nil {
		return setErr(fmt.Sprintf("CloseOverview: %v", err))
	}
	err = closeVersion()
	if err != nil {
		return setErr(fmt.Sprintf("CloseOverview: %v", err))
	}
	currentOverview = nil
	currentRepo = nil
	currentVersion = nil
	previousVersion = nil
	previousFilesMap = nil
	return rcOK
}

func RegisterNewRepositoryGo(repoName string) int64 {
	if currentOverview == nil {
		return setErr("RegisterNewRepository: can't register a repository without an open overview")
	}
	if currentOverview.GetPath(repoName) != nil {
		return setErr("RegisterNewRepository: name for new repo already taken")
	}

	err := closeRepo()
	if err != nil {
		return setErr(fmt.Sprintf(
			"RegisterNewRepository: error while trying to close previously loaded repo: %v", err))
	}
	err = closeVersion()
	if err != nil {
		return setErr(fmt.Sprintf(
			"RegisterNewRepository: error while trying to close previously loaded version: %v", err))
	}

	repoPath := timestampBase64()
	currentOverview.RegisterRepository(repoName, repoPath)
	currentRepo = &RepositoryManifest{
		RepositoryName: repoName,
		VersionNames:   make([]string, 0),
		VersionPaths:   make([]string, 0),
	}
	currentVersion = nil
	previousVersion = nil
	previousFilesMap = nil
	return rcOK
}

func LoadRepositoryGo(repoName string) int64 {
	if currentOverview == nil {
		return setErr("LoadRepository: can't register a repository without an open overview")
	}
	if currentOverview.GetPath(repoName) == nil {
		return setErr(fmt.Sprintf("LoadRepository: repo with given name '%s' does not exist", repoName))
	}

	err := closeRepo()
	if err != nil {
		return setErr(fmt.Sprintf(
			"LoadRepository: error while trying to close previously loaded repo: %v", err))
	}
	err = closeVersion()
	if err != nil {
		return setErr(fmt.Sprintf(
			"LoadRepository: error while trying to close previously loaded version: %v", err))
	}

	currentRepo = &RepositoryManifest{
		RepositoryName: repoName,
	}
	err = currentRepo.StreamFromFile(getRepoPath())
	currentVersion = nil
	if err != nil {
		currentRepo = nil
		return setErr(fmt.Sprintf("LoadRepository: failed while trying to load specified repo: %v", err))
	}
	return rcOK
}

func RegisterNewVersionGo(versionName string) int64 {
	if currentOverview == nil || currentRepo == nil {
		return setErr("RegisterNewVersion: can't register a version without an open overview + repo")
	}
	if currentRepo.GetPath(versionName) != nil {
		return setErr("RegisterNewVersion: version name already taken")
	}

	err := closeVersion()
	if err != nil {
		return setErr(fmt.Sprintf(
			"RegisterNewVersion: error while trying to close previously loaded version: %v", err))
	}

	versionPath := timestampBase64()
	currentVersion = &VersionManifest{
		VersionName:     versionPath,
		PreviousVersion: nil,
		BlobPath:        versionPath,
		Created:         uint64(time.Now().UnixNano()),
		Files:           make([]FileManifest, 0),
	}
	previousVersion = nil
	previousFilesMap = nil

	return rcOK
}

func LoadAndSetPreviousVersionGo(oldVersionName string) int64 {
	if currentVersion == nil {
		return setErr("LoadAndSetPreviousVersion: can't set a previous version on something that isn't loaded")
	}
	if currentRepo.GetPath(oldVersionName) == nil {
		return setErr("LoadAndSetPreviousVersion: given name for previous version could not be found")
	}
	previousVersion = &VersionManifest{}
	err := previousVersion.StreamFromFile(getVersionPathFrom(oldVersionName))
	if err != nil {
		previousVersion = nil
		previousFilesMap = nil
		return setErr(fmt.Sprintf("LoadAndSetPreviousVersion: failed to load previous version: %v", err))
	}
	previousFilesMap = previousVersion.GetFileMap()
	currentVersion.PreviousVersion = &oldVersionName
	return rcOK
}

func StartWriteToVersionGo(compression *int64) int64 {
	if currentVersion == nil {
		return setErr("StartWriteToVersion: can't write to a version that isn't opened")
	}
	if len(currentVersion.Files) != 0 {
		return setErr("StartWriteToVersion: can only start writing if the version isn't already created")
	}
	return BlobOpenGo("", getVersionBlob(), compression)
}

func TryWritingToVersionGo(path string, position *uint64) (int64, bool) {
	if currentWriteFile == nil || currentVersion == nil {
		return setErr("TryWritingToVersion: failed to write to an unopened version or blob"), false
	}

	var fileLength uint64
	//In case the file is skipped keep the position the same
	filePosition := *position
	var fileLastModifiedNs uint64
	var prevHash *string = nil
	var fileChanged int64
	var blobPath = currentVersion.BlobPath

	if val := previousFilesMap[path]; val != nil {
		fileLength, filePosition, fileLastModifiedNs = val.FilePosition, val.FilePosition, val.FileTS
		prevHashString := val.FileHash
		prevHash = &prevHashString
		blobPath = val.BlobPath
	}

	retCode, newHash := BlobCompressGo(path, &fileLength, &filePosition, &fileLastModifiedNs, prevHash, &fileChanged)
	if retCode != rcOK {
		return retCode, false
	}

	if fileChanged == FileUnchanged {
		currentVersion.Files = append(currentVersion.Files, FileManifest{
			FilePath:     path,
			FileLength:   fileLength,
			FilePosition: filePosition,
			FileHash:     newHash,
			FileTS:       fileLastModifiedNs,
			BlobPath:     blobPath,
		})
	} else if fileChanged == FileChanged {
		currentVersion.Files = append(currentVersion.Files, FileManifest{
			FilePath:     path,
			FileLength:   fileLength,
			FilePosition: filePosition,
			FileHash:     newHash,
			FileTS:       fileLastModifiedNs,
			BlobPath:     currentVersion.BlobPath,
		})
		// Only update position, when the file is actually written to the blob
		*position = filePosition
	}

	return rcOK, fileChanged == FileChanged
}

func StopWriteToVersionGo() (int64, string) {
	if currentVersion == nil {
		return setErr("StopWriteToVersion: can't stop writing to a version that isn't opened"), ""
	}
	currentVersion.SortFiles()
	return BlobCloseWithStatisticsGo()
}

func ReadFromVersionGo(
	matches []string,
	overwriteExistingFiles bool,
	callback *func(filesWritten int64, bytesWritten uint64), // being called after every processed file
) int64 {
	if currentVersion == nil {
		return setErr("ReadFromVersion: can't read from a version that isn't loaded")
	}
	if len(currentVersion.Files) == 0 {
		return rcOK
	}
	currBlob := ""
	var filesWritten int64 = 0
	var bytesWritten uint64 = 0

	for _, file := range currentVersion.Files {
		_, err := os.Stat(file.FilePath)
		if (!overwriteExistingFiles && os.IsExist(err)) || !matchAnyHandle(file.FilePath, matches) {
			if callback != nil {
				(*callback)(filesWritten, bytesWritten)
			}
			continue
		}

		if file.BlobPath != currBlob {
			if currentReadFile != nil {
				if err = currentReadFile.close(); err != nil {
					return setErr(fmt.Sprintf("ReadFromVersion: failed to close previous blob: %v", err))
				}
			}
			currBlob = file.BlobPath
			err, currentReadFile = openFileRead(getVersionBlobWithBlobName(currBlob))
			if err != nil {
				return setErr(fmt.Sprintf("ReadFromVersion: failed to open next blob: %v", err))
			}
		}
		position, length := file.FilePosition, file.FileLength
		err = currentReadFile.decompressFile(file.FilePath, &position, length)
		if err != nil {
			return setErr(fmt.Sprintf("ReadFromVersion: failed to decompress file: %v", err))
		}

		filesWritten += 1
		bytesWritten += length

		if callback != nil {
			(*callback)(filesWritten, bytesWritten)
		}
	}

	if currentReadFile != nil {
		if err := currentReadFile.close(); err != nil {
			return setErr(fmt.Sprintf("ReadFromVersion: failed to close last blob: %v", err))
		}
	}
	return rcOK
}

// -----------------------------------------------------------------------------
// HELPER FUNCTIONS
// -----------------------------------------------------------------------------

func matchAnyHandle(path string, pattern []string) bool {
	for _, pat := range pattern {
		success, _ := filepath.Match(pat, path)
		if success {
			return true
		}
	}
	return false
}

func closeRepo() error {
	if currentRepo == nil {
		return nil
	}
	return currentRepo.StreamToFile(getRepoPath())
}

func getRepoPath() string {
	return path.Join(currentOverviewFolder,
		*currentOverview.GetPath(currentRepo.RepositoryName)) +
		RepositorySuffix
}

func closeVersion() error {
	if currentVersion == nil {
		return nil
	}
	return currentVersion.StreamToFile(getVersionPath())
}

func getVersionPath() string {
	return getVersionPathFrom(currentVersion.VersionName)
}

func getVersionPathFrom(versionName string) string {
	return path.Join(currentOverviewFolder,
		*currentOverview.GetPath(currentRepo.RepositoryName)) +
		"_" +
		*currentRepo.GetPath(versionName) +
		VersionSuffix
}

func getVersionBlob() string {
	return path.Join(currentOverviewFolder,
		*currentOverview.GetPath(currentRepo.RepositoryName)) +
		"_" +
		*currentRepo.GetPath(currentVersion.VersionName) +
		BlobSuffix
}

func getVersionBlobWithBlobName(blobName string) string {
	return path.Join(currentOverviewFolder,
		*currentOverview.GetPath(currentRepo.RepositoryName)) +
		"_" +
		blobName +
		BlobSuffix
}

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

var timeEncoder = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-+").WithPadding('#')

func timestampBase64() string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(time.Now().UnixNano()))
	return timeEncoder.EncodeToString(buf)
}
