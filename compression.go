package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

type CompressionWriter struct {
	in       *os.File
	enc      *zstd.Encoder
	position uint64
	filePath string
}

func openFileWrite(path string, level int) (error, *CompressionWriter) {
	if level == -1 {
		level = 6
	}
	f, err := os.Create(path)
	if err != nil {
		return err, nil
	}
	enc, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
	if err != nil {
		return err, nil
	}
	obj := CompressionWriter{
		in:       f,
		enc:      enc,
		position: 0,
		filePath: path,
	}
	return nil, &obj
}

func (c *CompressionWriter) compressFileWithHash(path string) (error, uint64, string) {
	f, err := os.Open(path)
	if err != nil {
		return err, 0, ""
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, ChunkSize)
	var total uint64

	for {
		var rErr error
		var n int
		n, rErr = f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			c.enc.Write(buf[:n])
			total += uint64(n)
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return rErr, total, ""
		}
	}
	return nil, total, hex.EncodeToString(h.Sum(nil))
}

func (c *CompressionWriter) close() error {
	err := c.enc.Close()
	if err != nil {
		return err
	}
	return c.in.Close()
}

type DecompressionWriter struct {
	in       *os.File
	dec      *zstd.Decoder
	position uint64
}

func openFileRead(path string) (error, *DecompressionWriter) {
	f, err := os.Open(path)
	if err != nil {
		return err, nil
	}
	dec, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return err, nil
	}
	obj := DecompressionWriter{
		in:       f,
		dec:      dec,
		position: 0,
	}
	return nil, &obj
}

func (d *DecompressionWriter) decompressFile(targetPath string, position *uint64, fileLength uint64) error {
	var targetPos = *position
	if targetPos < d.position {
		return fmt.Errorf("target start position is smaller current file position")
	}
	var err error
	var f *os.File

	if err = os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to open create file directory: %v", err)
	}
	f, err = os.Create(targetPath)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("failed to open target file: %v", err))
	}
	defer f.Close()

	buf := make([]byte, ChunkSize)
	var total uint64
	remaining := fileLength
	firstToRead := targetPos - d.position

	for firstToRead > 0 {
		want := uint64(ChunkSize)
		if firstToRead < want {
			want = firstToRead
		}
		nr, readErr := d.dec.Read(buf[:want])
		if readErr == io.EOF {
			return fmt.Errorf(
				"read EOF early before the leading %d bytes could be read: %v",
				firstToRead, readErr)
		}
		firstToRead -= uint64(nr)
		d.position += uint64(nr)
	}

	for remaining > 0 {
		want := uint64(ChunkSize)
		if remaining < want {
			want = remaining
		}
		nr, readErr := d.dec.Read(buf[:want])
		if readErr == io.EOF {
			return fmt.Errorf(
				"read EOF early, only %d/%d bytes could be read: %v",
				fileLength-remaining, fileLength, readErr)
		}
		f.Write(buf[:nr])

		total += uint64(nr)
		d.position += uint64(nr)
		remaining -= uint64(nr)
	}

	*position = d.position
	return nil
}

func (d *DecompressionWriter) close() error {
	d.dec.Close()
	return d.in.Close()
}

func ReadFromFile(path string, readerFunc func(reader io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return err
	}
	defer dec.Close()
	return readerFunc(dec)
}

func ReadToFile(path string, writerFunc func(reader io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	if err != nil {
		return err
	}
	defer enc.Close()
	return writerFunc(enc)
}
