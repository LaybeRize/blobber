package main

import (
	"cmp"
	"encoding/json"
	"io"
	"slices"
)

type FileManifest struct {
	FilePath     string `json:"filePath"`
	FileLength   uint64 `json:"fileLength"`
	FilePosition uint64 `json:"filePosition"`
	FileHash     string `json:"fileHash"`
	FileTS       uint64 `json:"fileTS"`
	BlobPath     string `json:"blobPath"`
}

type VersionManifest struct {
	VersionName     string         `json:"versionName"`
	PreviousVersion *string        `json:"previousVersion"`
	BlobPath        string         `json:"blobPath"`
	Created         uint64         `json:"created"`
	Files           []FileManifest `json:"files"`
}

func (r *VersionManifest) SortFiles() {
	slices.SortFunc(r.Files, func(a, b FileManifest) int {
		if a.BlobPath != b.BlobPath {
			return cmp.Compare(a.BlobPath, b.BlobPath)
		}
		return cmp.Compare(a.FilePosition, b.FilePosition)
	})
}

func (r *VersionManifest) StreamToFile(path string) error {
	return ReadToFile(path, func(writer io.Writer) error {
		return json.NewEncoder(writer).Encode(r)
	})
}

func (r *VersionManifest) StreamFromFile(path string) error {
	return ReadFromFile(path, func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(r)
	})
}

func (r *VersionManifest) GetFileMap() map[string]*FileManifest {
	res := make(map[string]*FileManifest)
	for _, m := range r.Files {
		res[m.FilePath] = &m
	}
	return res
}

type RepositoryManifest struct {
	RepositoryName string   `json:"repositoryName"`
	VersionNames   []string `json:"versionNames"`
	VersionPaths   []string `json:"versionPaths"`
}

func (r *RepositoryManifest) GetPath(name string) *string {
	if pos := slices.Index(r.VersionNames, name); pos != -1 {
		return &(r.VersionPaths[pos])
	}
	return nil
}

func (r *RepositoryManifest) StreamToFile(path string) error {
	return ReadToFile(path, func(writer io.Writer) error {
		return json.NewEncoder(writer).Encode(r)
	})
}

func (r *RepositoryManifest) StreamFromFile(path string) error {
	return ReadFromFile(path, func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(r)
	})
}

type RepositoryOverview struct {
	RepositoryNames []string `json:"repositoryNames"`
	RepositoryPaths []string `json:"repositoryPaths"`
}

func (r *RepositoryOverview) GetPath(name string) *string {
	if pos := slices.Index(r.RepositoryNames, name); pos != -1 {
		return &(r.RepositoryPaths[pos])
	}
	return nil
}

func (r *RepositoryOverview) RegisterRepository(name string, path string) {
	r.RepositoryNames = append(r.RepositoryNames, name)
	r.RepositoryPaths = append(r.RepositoryPaths, path)
}

func (r *RepositoryOverview) StreamToFile(path string) error {
	return ReadToFile(path, func(writer io.Writer) error {
		return json.NewEncoder(writer).Encode(r)
	})
}

func (r *RepositoryOverview) StreamFromFile(path string) error {
	return ReadFromFile(path, func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(r)
	})
}
