/* new archive.go file */
package main

import (
	"encoding/json"
	"io"
)

type ArchiveFileManifest struct {
	GroupName        string `json:"groupName"`
	RelativeFilePath string `json:"filePath"`
	FileLength       uint64 `json:"fileLength"`
	FilePosition     uint64 `json:"filePosition"`
}

type ArchiveOverview struct {
	ArchiveName string                `json:"archiveName"`
	Creator     string                `json:"creator"`
	Path        string                `json:"path"`
	Groups      []string              `json:"groups"`
	Files       []ArchiveFileManifest `json:"files"`
}

func (r *ArchiveOverview) StreamToFile(path string) error {
	return ReadToFile(path, func(writer io.Writer) error {
		return json.NewEncoder(writer).Encode(r)
	})
}

func (r *ArchiveOverview) StreamFromFile(path string) error {
	return ReadFromFile(path, func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(r)
	})
}
