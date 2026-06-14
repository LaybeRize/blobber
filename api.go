package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	SkippedFile int64 = iota - 1
	FileUnchanged
	FileChanged

	ChunkSize = 4 * 1024 * 1024

	rcOK  int64 = 1
	rcErr int64 = 0
)

var Divider int64 = 20
var ByteMarker uint64 = 64 * 1024 * 1024
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
	fileLastModifiedNs *int64,
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
	curMtime := info.ModTime().UnixNano()

	isFirst := prevHash == nil
	var newHash string
	var written uint64

	if !isFirst {
		// Cheap check: length + mtime first
		if curLen == *fileLength && curMtime == *fileLastModifiedNs {
			*fileChanged = FileUnchanged
			return rcOK, *prevHash
		}
		var herr error
		newHash, herr = hashFile(filePath)
		if herr != nil {
			return setErr(fmt.Sprintf("BlobCompress: hash %q: %v", filePath, herr)), ""
		}
		if newHash == *prevHash {
			*fileChanged = FileUnchanged
			return rcOK, newHash
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

func BlobTryCloseGo() int64 {
	if currentWriteFile != nil || currentReadFile != nil {
		return BlobCloseGo()
	}
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
	if err := os.MkdirAll(overviewFolder, 0755); err != nil {
		return setErr(fmt.Sprintf("LoadOverviewGo: failed to create file directory '%s' : %v",
			overviewFolder, err))
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
	currentRepo.RegisterVersion(versionName, versionPath)
	currentVersion = &VersionManifest{
		VersionName:     versionName,
		PreviousVersion: nil,
		BlobPath:        versionPath,
		Created:         uint64(time.Now().UnixNano()),
		Files:           make([]FileManifest, 0),
	}
	previousVersion = nil
	previousFilesMap = nil

	return rcOK
}

func LoadVersionGo(versionName string) int64 {
	if currentOverview == nil || currentRepo == nil {
		return setErr("LoadVersion: can't register a version without an open overview + repo")
	}
	if currentRepo.GetPath(versionName) == nil {
		return setErr("LoadVersion: version name could not be found")
	}

	err := closeVersion()
	if err != nil {
		return setErr(fmt.Sprintf(
			"LoadVersion: error while trying to close previously loaded version: %v", err))
	}

	currentVersion = &VersionManifest{
		VersionName: versionName,
	}
	err = currentVersion.StreamFromFile(getVersionPath())
	if err != nil {
		return setErr(fmt.Sprintf(
			"LoadVersion: error while trying to load newly selected version: %v", err))
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

func GetVersionInfoGo() (int64, string) {
	if currentVersion == nil {
		return setErr("GetVersionInfo: can't return version info without version."), ""
	}
	return rcOK, fmt.Sprintf("Name: %s\nCreated: %s\nFiles: %d",
		currentVersion.VersionName,
		time.Unix(0, int64(currentVersion.Created)).UTC().In(time.Local).Format(time.RFC3339),
		len(currentVersion.Files))
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

func TryWritingToVersionGo(path string, position *uint64, bytesProcessed *uint64) (int64, bool) {
	if currentWriteFile == nil || currentVersion == nil {
		return setErr("TryWritingToVersion: failed to write to an unopened version or blob"), false
	}

	var fileLength uint64
	//In case the file is skipped keep the position the same
	filePosition := *position
	var fileLastModifiedNs int64
	var prevHash *string = nil
	var fileChanged int64
	var blobPath = currentVersion.BlobPath

	if val := previousFilesMap[path]; val != nil {
		fileLength, filePosition, fileLastModifiedNs = val.FileLength, val.FilePosition, val.FileTS
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
		*bytesProcessed = filePosition + fileLength
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
	callback func(filesProcessed int64, filesWritten int64, bytesWritten uint64), // being called after every processed file
) int64 {
	if currentVersion == nil {
		return setErr("ReadFromVersion: can't read from a version that isn't loaded")
	}
	if len(currentVersion.Files) == 0 {
		callback(0, 0, 0)
		return rcOK
	}
	currBlob := ""
	localWrapFiles := (int64(len(currentVersion.Files)) / Divider) + 1
	localNextBytesStep := ByteMarker
	var filesWritten int64 = 0
	var filesProcessed int64 = 0
	var bytesWritten uint64 = 0

	for _, file := range currentVersion.Files {
		// Do it this way so the last callback() function call has at least one variable that is different
		// from the last call from here.
		filesProcessed += 1
		if filesProcessed%localWrapFiles == 0 {
			if bytesWritten > localNextBytesStep {
				localNextBytesStep = bytesWritten + ByteMarker
			}
			callback(filesProcessed-1, filesWritten, bytesWritten)
		} else if bytesWritten > localNextBytesStep {
			localNextBytesStep = bytesWritten + ByteMarker
			callback(filesProcessed-1, filesWritten, bytesWritten)
		}

		_, err := os.Stat(file.FilePath)
		if (!overwriteExistingFiles && os.IsExist(err)) || (len(matches) != 0 && !matchAnyHandle(file.FilePath, matches)) {
			continue
		}

		if file.BlobPath != currBlob {
			if retCode := BlobTryCloseGo(); retCode != rcOK {
				return retCode
			}
			currBlob = file.BlobPath
			if retCode := BlobOpenGo(getVersionBlobWithBlobName(currBlob), "", nil); retCode != rcOK {
				return retCode
			}
		}

		position, length := file.FilePosition, file.FileLength
		retCode := BlobDecompressGo(file.FilePath, &position, length)
		if retCode != rcOK {
			return retCode
		}

		filesWritten += 1
		bytesWritten += length
	}

	callback(filesProcessed, filesWritten, bytesWritten)

	if retCode := BlobTryCloseGo(); retCode != rcOK {
		return retCode
	}
	return rcOK
}

func EstimateReadGo(
	matches []string,
	overwriteExistingFiles bool,
) (int64, []string) {
	if currentVersion == nil {
		return setErr("ReadFromVersion: can't read from a version that isn't loaded"), nil
	}
	res := make([]string, len(currentVersion.Files))
	counter := 0
	for _, file := range currentVersion.Files {
		_, err := os.Stat(file.FilePath)
		if (!overwriteExistingFiles && !errors.Is(err, fs.ErrNotExist)) || (len(matches) != 0 && !matchAnyHandle(file.FilePath, matches)) {
			continue
		}
		res[counter] = file.FilePath
		counter += 1
	}
	return rcOK, res[:counter]
}

// -----------------------------------------------------------------------------
// ARCHIVE FUNCTIONS
// -----------------------------------------------------------------------------

const (
	ArchiveOverviewName = "archive.overview"
	ArchiveBlobName     = "archive.blob"
)

var currentArchive *ArchiveOverview = nil

func CreateArchiveGo(name string, folder string) int64 {
	if currentArchive != nil {
		return setErr("CreateArchive: an archive is already open, close it first")
	}

	if err := os.MkdirAll(folder, 0755); err != nil {
		return setErr(fmt.Sprintf("CreateArchive: failed to create archive folder '%s': %v", folder, err))
	}

	currentArchive = &ArchiveOverview{
		ArchiveName: name,
		Path:        folder,
		Groups:      make([]string, 0),
		Files:       make([]ArchiveFileManifest, 0),
	}

	blobPath := path.Join(folder, ArchiveBlobName)
	if retCode := BlobOpenGo("", blobPath, nil); retCode != rcOK {
		currentArchive = nil
		return setErr(fmt.Sprintf("CreateArchive: failed to open blob for writing: %v", errorMsg))
	}

	return rcOK
}

func AddNewGroupGo(groupName string, pathPrefix string, paths []string) int64 {
	if currentArchive == nil {
		return setErr("AddNewGroup: no archive is currently open")
	}

	for _, group := range currentArchive.Groups {
		if group == groupName {
			return setErr(fmt.Sprintf("AddNewGroup: group '%s' already exists in archive", groupName))
		}
	}

	newFiles := make([]ArchiveFileManifest, 0, len(paths))

	for _, filePath := range paths {
		if !strings.HasPrefix(filePath, pathPrefix) {
			return setErr(fmt.Sprintf(
				"AddNewGroup: path '%s' does not have the required prefix '%s'", filePath, pathPrefix))
		}

		relativePath := strings.TrimPrefix(filePath, pathPrefix)

		var fileLength uint64
		var filePosition = currentWriteFile.position
		var fileLastModifiedNs int64
		var fileChanged int64

		retCode, _ := BlobCompressGo(filePath, &fileLength, &filePosition, &fileLastModifiedNs, nil, &fileChanged)
		if retCode != rcOK {
			return retCode
		}

		if fileChanged == SkippedFile {
			continue
		}

		newFiles = append(newFiles, ArchiveFileManifest{
			GroupName:        groupName,
			RelativeFilePath: relativePath,
			FileLength:       fileLength,
			FilePosition:     filePosition,
		})

		currentWriteFile.position += fileLength
	}

	currentArchive.Groups = append(currentArchive.Groups, groupName)
	currentArchive.Files = append(currentArchive.Files, newFiles...)

	return rcOK
}

func LoadArchiveGo(folder string) (int64, []string) {
	if currentArchive != nil {
		return setErr("LoadArchive: an archive is already open, close it first"), nil
	}

	overviewPath := path.Join(folder, ArchiveOverviewName)

	currentArchive = &ArchiveOverview{}
	if err := currentArchive.StreamFromFile(overviewPath); err != nil {
		currentArchive = nil
		return setErr(fmt.Sprintf("LoadArchive: failed to load archive overview from '%s': %v",
			overviewPath, err)), nil
	}

	blobPath := path.Join(folder, ArchiveBlobName)
	if retCode := BlobOpenGo(blobPath, "", nil); retCode != rcOK {
		currentArchive = nil
		return setErr(fmt.Sprintf("LoadArchive: failed to open blob for reading: %v", errorMsg)), nil
	}

	return rcOK, currentArchive.Groups
}

// ReadArchiveGo decompresses files from the archive using the provided prefix mapping.
// Groups with a nil pointer value in the map are skipped entirely.
// For included groups, the output path is: *mappedPrefix + RelativeFilePath
func ReadArchiveGo(prefixMapping map[string]*string) int64 {
	if currentArchive == nil {
		return setErr("ReadArchive: no archive is currently open")
	}

	for _, file := range currentArchive.Files {
		prefix, exists := prefixMapping[file.GroupName]
		if !exists || prefix == nil {
			continue
		}

		targetPath := path.Join(*prefix, file.RelativeFilePath)
		position := file.FilePosition

		if retCode := BlobDecompressGo(targetPath, &position, file.FileLength); retCode != rcOK {
			return retCode
		}
	}

	return rcOK
}

func CloseArchiveGo() int64 {
	if currentArchive == nil {
		return setErr("CloseArchive: no archive is currently open")
	}

	overviewPath := path.Join(currentArchive.Path, ArchiveOverviewName)
	if err := currentArchive.StreamToFile(overviewPath); err != nil {
		return setErr(fmt.Sprintf("CloseArchive: failed to save archive overview: %v", err))
	}

	if retCode := BlobCloseGo(); retCode != rcOK {
		return retCode
	}

	currentArchive = nil
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
