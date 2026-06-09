//go:build cli

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------
// ENTRY POINT
// -----------------------------------------------------------------------------

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	// Direct blob commands
	case "compress":
		cmdCompress(args)
	case "decompress":
		cmdDecompress(args)

	// Repository management (non-interactive)
	case "repo":
		cmdRepo(args)

	// Interactive versioning session
	case "version":
		cmdVersionSession(args)

	case "help", "--help", "-h":
		printUsage()

	default:
		fatalf("unknown command %q — run 'blobber help'\n", cmd)
	}
}

// -----------------------------------------------------------------------------
// COMPRESS  blobber compress <blob-file> [--level N] <path|file>...
// -----------------------------------------------------------------------------

func cmdCompress(args []string) {
	if len(args) < 2 {
		fatalf("usage: blobber compress <blob-file> [--level N] <path|file>...\n")
	}

	blobFile := args[0]
	args = args[1:]

	var level *int64
	var inputs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--level" {
			if i+1 >= len(args) {
				fatalf("--level requires a value\n")
			}
			i++
			n, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				fatalf("invalid level %q: %v\n", args[i], err)
			}
			level = &n
		} else {
			inputs = append(inputs, args[i])
		}
	}

	files, err := resolveInputs(inputs)
	dieOnErr(err)

	if BlobOpenGo("", blobFile, level) != rcOK {
		fatalf("open blob for writing: %s\n", errorMsg)
	}

	var totalFiles, writtenFiles int64
	var totalBytes uint64
	var pos uint64

	for _, f := range files {
		var length uint64
		var mtime int64
		var changed int64
		code, _ := BlobCompressGo(f, &length, &pos, &mtime, nil, &changed)
		if code != rcOK {
			fatalf("compress %q: %s\n", f, errorMsg)
		}
		totalFiles++
		if changed == FileChanged {
			writtenFiles++
			totalBytes += length
			pos += length
		} else if changed == SkippedFile {
			totalFiles-- // not a regular file, don't count
		}
		printProgress(totalFiles, writtenFiles, totalBytes)
	}

	code, stat := BlobCloseWithStatisticsGo()
	if code != rcOK {
		fatalf("close blob: %s\n", errorMsg)
	}
	fmt.Printf("\nDone. Compression ratio: %s\n", stat)
}

// -----------------------------------------------------------------------------
// DECOMPRESS  blobber decompress <blob-file> <target-path> <position> <length>
// -----------------------------------------------------------------------------

func cmdDecompress(args []string) {
	if len(args) != 4 {
		fatalf("usage: blobber decompress <blob-file> <target-path> <position> <length>\n")
	}
	blobFile := args[0]
	targetPath := args[1]
	pos, err1 := strconv.ParseUint(args[2], 10, 64)
	length, err2 := strconv.ParseUint(args[3], 10, 64)
	if err1 != nil || err2 != nil {
		fatalf("position and length must be unsigned integers\n")
	}

	if BlobOpenGo(blobFile, "", nil) != rcOK {
		fatalf("open blob for reading: %s\n", errorMsg)
	}
	if BlobDecompressGo(targetPath, &pos, length) != rcOK {
		fatalf("decompress: %s\n", errorMsg)
	}
	if BlobCloseGo() != rcOK {
		fatalf("close blob: %s\n", errorMsg)
	}
	fmt.Println("Decompressed successfully.")
}

// -----------------------------------------------------------------------------
// REPO  blobber repo <store-dir> <sub-command> [args...]
//
//   blobber repo <dir> list
//   blobber repo <dir> new  <repo-name>
//   blobber repo <dir> load <repo-name> list-versions
// -----------------------------------------------------------------------------

func cmdRepo(args []string) {
	if len(args) < 2 {
		fatalf("usage: blobber repo <store-dir> list|new|load ...\n")
	}
	storeDir := args[0]
	sub := args[1]
	rest := args[2:]

	openOverview(storeDir)

	switch sub {
	case "list":
		if currentOverview == nil || len(currentOverview.RepositoryNames) == 0 {
			fmt.Println("(no repositories)")
			return
		}
		for _, name := range currentOverview.RepositoryNames {
			fmt.Println(" •", name)
		}

	case "new":
		if len(rest) < 1 {
			fatalf("usage: blobber repo <dir> new <name>\n")
		}
		if RegisterNewRepositoryGo(rest[0]) != rcOK {
			fatalf("create repository: %s\n", errorMsg)
		}
		closeOverview()
		fmt.Printf("Repository %q created.\n", rest[0])

	case "load":
		if len(rest) < 2 {
			fatalf("usage: blobber repo <dir> load <name> list-versions\n")
		}
		if LoadRepositoryGo(rest[0]) != rcOK {
			fatalf("load repository: %s\n", errorMsg)
		}
		switch rest[1] {
		case "list-versions":
			if len(currentRepo.VersionNames) == 0 {
				fmt.Println("(no versions)")
			}
			for _, v := range currentRepo.VersionNames {
				fmt.Println(" •", v)
			}
		default:
			fatalf("unknown repo sub-command %q\n", rest[1])
		}
		closeOverview()

	default:
		fatalf("unknown repo sub-command %q\n", sub)
	}
}

// -----------------------------------------------------------------------------
// VERSION SESSION  blobber version <store-dir> [<repo-name>]
//
// Keeps the overview + repo loaded across multiple interactive commands so
// the user can manage versions without reloading state on every call.
// -----------------------------------------------------------------------------

func cmdVersionSession(args []string) {
	if len(args) < 1 {
		fatalf("usage: blobber version <store-dir> [<repo-name>]\n")
	}
	storeDir := args[0]

	openOverview(storeDir)

	// Optionally pre-load a repo
	if len(args) >= 2 {
		if LoadRepositoryGo(args[1]) != rcOK {
			fatalf("load repository %q: %s\n", args[1], errorMsg)
		}
		fmt.Printf("Loaded repository %q (%d version(s))\n", args[1], len(currentRepo.VersionNames))
	}

	fmt.Println(`Blobber version session started. Type 'help' for commands, 'exit' to save and quit.`)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(sessionPrompt())
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := splitArgs(line)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "help":
			printSessionHelp()

		case "exit", "quit":
			closeOverview()
			fmt.Println("Session saved. Goodbye.")
			return

		case "repos":
			if currentOverview == nil || len(currentOverview.RepositoryNames) == 0 {
				fmt.Println("  (no repositories)")
			} else {
				for _, r := range currentOverview.RepositoryNames {
					fmt.Println(" •", r)
				}
			}

		case "new-repo":
			if len(parts) < 2 {
				sessionErr("usage: new-repo <name>")
				continue
			}
			if RegisterNewRepositoryGo(parts[1]) != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Printf("  Repository %q created and loaded.\n", parts[1])

		case "use-repo":
			if len(parts) < 2 {
				sessionErr("usage: use-repo <name>")
				continue
			}
			if LoadRepositoryGo(parts[1]) != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Printf("  Repository %q loaded (%d version(s))\n", parts[1], len(currentRepo.VersionNames))

		case "versions":
			if currentRepo == nil {
				sessionErr("no repository loaded — use 'use-repo' first")
				continue
			}
			if len(currentRepo.VersionNames) == 0 {
				fmt.Println("  (no versions)")
			} else {
				for _, v := range currentRepo.VersionNames {
					fmt.Println(" •", v)
				}
			}

		case "new-version":
			if len(parts) < 2 {
				sessionErr("usage: new-version <name>")
				continue
			}
			if RegisterNewVersionGo(parts[1]) != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Printf("  Version %q created and active.\n", parts[1])

		case "use-version":
			if len(parts) < 2 {
				sessionErr("usage: use-version <name>")
				continue
			}
			if LoadVersionGo(parts[1]) != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Printf("  Version %q loaded (%d file(s))\n", parts[1], len(currentVersion.Files))

		case "set-base":
			// set-base <previous-version-name>
			if len(parts) < 2 {
				sessionErr("usage: set-base <previous-version-name>")
				continue
			}
			if LoadAndSetPreviousVersionGo(parts[1]) != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Printf("  Base version set to %q (delta compression active).\n", parts[1])

		case "write":
			// write [--level N] [--from-file <file>] <path|dir>...
			if currentVersion == nil {
				sessionErr("no version active — create or load one first")
				continue
			}
			sessionWrite(parts[1:])

		case "read":
			// read [--overwrite] [--from-file <file>] <glob-pattern>...
			if currentVersion == nil {
				sessionErr("no version active — load one first")
				continue
			}
			sessionRead(parts[1:], false)

		case "estimate":
			// estimate [--overwrite] [--from-file <file>] <glob-pattern>...
			if currentVersion == nil {
				sessionErr("no version active — load one first")
				continue
			}
			sessionRead(parts[1:], true)

		case "list-files":
			if currentVersion == nil {
				sessionErr("no version active — load one first")
				continue
			}
			if len(currentVersion.Files) == 0 {
				fmt.Println("  (version is empty)")
			} else {
				for _, f := range currentVersion.Files {
					fmt.Printf("  %s  (%s)\n", f.FilePath, formatBytes(f.FileLength))
				}
			}

		case "status":
			printStatus()

		default:
			sessionErr(fmt.Sprintf("unknown command %q — type 'help'", parts[0]))
		}
	}

	// Ctrl-D: save cleanly
	closeOverview()
	fmt.Println("\nSession saved.")
}

// sessionWrite handles: write [--level N] [--from-file <file>] <path|dir>...
func sessionWrite(args []string) {
	var level *int64
	var inputs []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--level":
			if i+1 >= len(args) {
				sessionErr("--level requires a value")
				return
			}
			i++
			n, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				sessionErr(fmt.Sprintf("invalid level %q", args[i]))
				return
			}
			level = &n
		case "--from-file":
			if i+1 >= len(args) {
				sessionErr("--from-file requires a path")
				return
			}
			i++
			lines, err := readLines(args[i])
			if err != nil {
				sessionErr(err.Error())
				return
			}
			inputs = append(inputs, lines...)
		default:
			inputs = append(inputs, args[i])
		}
	}

	files, err := resolveInputs(inputs)
	if err != nil {
		sessionErr(err.Error())
		return
	}

	if len(files) == 0 {
		sessionErr("no files resolved from provided inputs")
		return
	}

	if StartWriteToVersionGo(level) != rcOK {
		sessionErr(errorMsg)
		return
	}

	var pos uint64
	var bytesProcessed uint64
	var totalFiles, savedFiles int64

	for _, f := range files {
		code, saved := TryWritingToVersionGo(f, &pos, &bytesProcessed)
		if code != rcOK {
			// StopWriteToVersionGo still needs to run to close the blob
			StopWriteToVersionGo()
			sessionErr(errorMsg)
			return
		}
		totalFiles++
		if saved {
			savedFiles++
		}
		printProgress(totalFiles, savedFiles, bytesProcessed)
	}

	code, stat := StopWriteToVersionGo()
	if code != rcOK {
		sessionErr(errorMsg)
		return
	}
	fmt.Printf("\n  Written %d/%d file(s). Compression ratio: %s\n", savedFiles, totalFiles, stat)
}

// sessionRead handles: read|estimate [--overwrite] [--from-file <file>] <pattern>...
func sessionRead(args []string, estimateOnly bool) {
	var overwrite bool
	var patterns []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--overwrite":
			overwrite = true
		case "--from-file":
			if i+1 >= len(args) {
				sessionErr("--from-file requires a path")
				return
			}
			i++
			lines, err := readLines(args[i])
			if err != nil {
				sessionErr(err.Error())
				return
			}
			patterns = append(patterns, lines...)
		default:
			patterns = append(patterns, args[i])
		}
	}

	if estimateOnly {
		code, result := EstimateReadGo(patterns, overwrite)
		if code != rcOK {
			sessionErr(errorMsg)
			return
		}
		if len(result) == 0 {
			fmt.Println("  (nothing would be written)")
		} else {
			for _, p := range result {
				fmt.Println(" •", p)
			}
			fmt.Printf("  %d file(s) would be extracted.\n", len(result))
		}
		return
	}

	var totalFiles, writtenFiles int64
	var totalBytes uint64

	code := ReadFromVersionGo(patterns, overwrite, func(processed, written int64, bytes uint64) {
		totalFiles = processed
		writtenFiles = written
		totalBytes = bytes
		printProgress(processed, written, bytes)
	})

	fmt.Println()
	if code != rcOK {
		sessionErr(errorMsg)
		return
	}
	fmt.Printf("  Extracted %d/%d file(s), %s total.\n", writtenFiles, totalFiles, formatBytes(totalBytes))
}

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

// resolveInputs expands each argument: if it's a directory, walk it and collect
// all regular files; if it looks like a newline-delimited file (--from-file is
// the explicit flag, but we also accept a plain text file), treat it as a list.
// Everything else is treated as a literal file path.
func resolveInputs(inputs []string) ([]string, error) {
	var files []string
	seen := map[string]bool{}

	add := func(p string) {
		clean := filepath.Clean(p)
		if !seen[clean] {
			seen[clean] = true
			files = append(files, clean)
		}
	}

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("cannot stat %q: %w", input, err)
		}

		if info.IsDir() {
			err = filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.Type().IsRegular() {
					add(path)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("walking %q: %w", input, err)
			}
		} else {
			add(input)
		}
	}
	return files, nil
}

// readLines reads a newline-delimited file of paths (ignoring blank lines and
// lines starting with #).
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open path list %q: %w", path, err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, sc.Err()
}

func openOverview(dir string) {
	if LoadOverviewGo(dir) != rcOK {
		fatalf("open store at %q: %s\n", dir, errorMsg)
	}
}

func closeOverview() {
	if CloseOverviewGo() != rcOK {
		fmt.Fprintf(os.Stderr, "warning: close overview: %s\n", errorMsg)
	}
}

func sessionPrompt() string {
	repo := "-"
	version := "-"
	if currentRepo != nil {
		repo = currentRepo.RepositoryName
	}
	if currentVersion != nil {
		version = currentVersion.VersionName
	}
	return fmt.Sprintf("blobber [%s/%s]> ", repo, version)
}

func printStatus() {
	fmt.Println("  Store:  ", currentOverviewFolder)
	if currentRepo != nil {
		fmt.Println("  Repo:   ", currentRepo.RepositoryName)
	} else {
		fmt.Println("  Repo:    (none)")
	}
	if currentVersion != nil {
		created := time.Unix(0, int64(currentVersion.Created)).Format(time.RFC3339)
		fmt.Printf("  Version: %s (created %s, %d file(s))\n",
			currentVersion.VersionName, created, len(currentVersion.Files))
		if currentVersion.PreviousVersion != nil {
			fmt.Printf("  Base:    %s\n", *currentVersion.PreviousVersion)
		}
	} else {
		fmt.Println("  Version: (none)")
	}
}

// printProgress prints a one-line overwriting progress indicator.
func printProgress(processed, written int64, bytes uint64) {
	fmt.Printf("\r  processed: %d  written: %d  bytes: %s      ",
		processed, written, formatBytes(bytes))
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// splitArgs splits a line on whitespace, respecting simple double-quoted strings.
func splitArgs(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func dieOnErr(err error) {
	if err != nil {
		fatalf("%v\n", err)
	}
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format, a...)
	os.Exit(1)
}

func sessionErr(msg string) {
	fmt.Fprintf(os.Stderr, "  error: %s\n", msg)
}

// -----------------------------------------------------------------------------
// HELP TEXT
// -----------------------------------------------------------------------------

func printUsage() {
	fmt.Print(`blobber — blob archive tool

Usage:
  blobber compress   <blob-file> [--level N] <path|dir>...
  blobber decompress <blob-file> <target-path> <position> <length>
  blobber repo       <store-dir> <sub-command> [args...]
  blobber version    <store-dir> [<repo-name>]

Direct blob commands:
  compress    Compress one or more files/directories into a blob.
              Pass directories to recursively compress all files inside them.
              --level N  Compression level (1–22, default 6).

  decompress  Extract a single entry from a blob by byte position and length.

Repository management (non-interactive):
  repo <dir> list
      List all repositories in the store.

  repo <dir> new <name>
      Create a new repository.

  repo <dir> load <name> list-versions
      List all versions of a repository.

Versioning session (interactive):
  version <store-dir> [<repo-name>]
      Start an interactive session. The store stays open between commands,
      so you can create/switch repos and versions without reloading state.
      Type 'help' inside the session for available commands.

Build tags:
  Build this CLI with:  go build -tags cli .
  Build the DLL with:   go build -buildmode=c-shared .
`)
}

func printSessionHelp() {
	fmt.Print(`
  Session commands:

  status                     Show current store, repo, and version.
  repos                      List all repositories.
  new-repo  <name>           Create a new repository (auto-loads it).
  use-repo  <name>           Load an existing repository.

  versions                   List versions of the loaded repository.
  new-version <name>         Create a new (empty) version (auto-loads it).
  use-version <name>         Load an existing version.
  set-base  <version-name>   Set a previous version as base for delta compression.

  list-files                 List all files recorded in the active version.

  write [--level N] [--from-file <list.txt>] <path|dir>...
      Compress files into the active version.
      Directories are walked recursively.
      --from-file reads newline-separated paths from a text file.
      --level N sets compression level (1–22).

  read [--overwrite] [--from-file <list.txt>] [<glob>...]
      Extract files from the active version.
      Without glob patterns, extracts everything.
      --overwrite replaces existing files.

  estimate [--overwrite] [--from-file <list.txt>] [<glob>...]
      Dry-run of 'read': shows which files would be extracted.

  exit / quit                Save the session and exit.

`)
}
