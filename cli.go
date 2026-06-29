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

	// Archive (non-interactive)
	case "archive":
		cmdArchive(args)

	// Archive (interactive)
	case "archive-session":
		cmdArchiveSession(args)

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

	code, stat := WriteToVersionGo(level, func(filesProcessed int64, filesWritten int64, bytesWritten uint64) {
		printProgress(filesProcessed, filesWritten, bytesWritten)
	}, files)
	if code != rcOK {
		sessionErr(errorMsg)
		return
	}
	fmt.Printf("\n  Versioned %d file(s). Compression ratio: %s\n", len(currentVersion.Files), stat)
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
// ARCHIVE blobber archive <sub-command> [args...]
//
//   blobber archive <folder> create <name> <creator>
//   blobber archive <folder> add-group <group-name> <path-prefix> <path|dir>...
//   blobber archive <folder> load
//   blobber archive <folder> extract [--skip <group>]... [--map <group>=<prefix>]...
// -----------------------------------------------------------------------------

func cmdArchive(args []string) {
	if len(args) < 2 {
		fatalf("usage: blobber archive <folder> create|add-group|load|extract ...\n")
	}

	folder := args[0]
	sub := args[1]
	rest := args[2:]

	switch sub {
	case "load":
		// blobber archive <folder> load  — prints the group list
		retCode, groups, name, creator := LoadArchiveGo(folder)
		if retCode != rcOK {
			fatalf("load archive: %s\n", errorMsg)
		}
		if retCode, _ = CloseArchiveGo(); retCode != rcOK {
			fatalf("close archive: %s\n", errorMsg)
		}
		fmt.Println("Archive Name: ", name)
		fmt.Println("Creator: ", creator)
		fmt.Println("Groups:")
		if len(groups) == 0 {
			fmt.Println("(no groups)")
		} else {
			for _, g := range groups {
				fmt.Println(" •", g)
			}
		}

	case "extract":
		// blobber archive <folder> extract [--skip <group>]... [--map <group>=<prefix>]...
		retCode, groups, _, _ := LoadArchiveGo(folder)
		if retCode != rcOK {
			fatalf("load archive for extraction: %s\n", errorMsg)
		}

		mapping := buildPrefixMapping(groups, rest)

		if ReadArchiveGo(mapping, func(filesProcessed int64, filesWritten int64, bytesWritten uint64) {}) != rcOK {
			fatalf("extract archive: %s\n", errorMsg)
		}
		if retCode, _ = CloseArchiveGo(); retCode != rcOK {
			fatalf("close archive: %s\n", errorMsg)
		}
		fmt.Println("Extraction complete.")

	default:
		fatalf("unknown archive sub-command %q — run 'blobber help'\n", sub)
	}
}

// -----------------------------------------------------------------------------
// ARCHIVE SESSION blobber archive-session <folder>
//
// Keeps the archive open across multiple interactive commands so the user can
// create an archive, add groups, and extract — without reloading state each time.
// -----------------------------------------------------------------------------

func cmdArchiveSession(args []string) {
	if len(args) < 1 {
		fatalf("usage: blobber archive-session <folder>\n")
	}

	folder := args[0]

	fmt.Printf("Blobber archive session started for folder %q.\n", folder)
	fmt.Println("Type 'help' for commands, 'exit' to save and quit.")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print(archiveSessionPrompt())
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
			printArchiveSessionHelp()

		case "exit", "quit":
			if currentArchive != nil {
				if retCode, _ := CloseArchiveGo(); retCode != rcOK {
					_, _ = fmt.Fprintf(os.Stderr, "warning: close archive: %s\n", errorMsg)
				} else {
					fmt.Println("Archive saved.")
				}
			}
			fmt.Println("Goodbye.")
			return

		case "status":
			printArchiveStatus(folder)

		case "create":
			// create <name> <creator>
			if len(parts) < 3 {
				sessionErr("usage: create <name> <creator>")
				continue
			}
			if currentArchive != nil {
				sessionErr("an archive is already open — close it first with 'exit' or 'close'")
				continue
			}
			if CreateArchiveGo(parts[1], parts[2], folder) != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Printf(" Archive %q created and open for writing.\n", parts[1])

		case "load":
			// load
			if currentArchive != nil {
				sessionErr("an archive is already open — close it first with 'close'")
				continue
			}
			retCode, groups, name, creator := LoadArchiveGo(folder)
			if retCode != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Printf(" Archive %q from %q loaded (%d group(s)).\n", name, creator, len(groups))
			if len(groups) > 0 {
				for _, g := range groups {
					fmt.Println("  •", g)
				}
			}

		case "groups":
			if currentArchive == nil {
				sessionErr("no archive open — use 'create' or 'load' first")
				continue
			}
			if len(currentArchive.Groups) == 0 {
				fmt.Println(" (no groups)")
			} else {
				for _, g := range currentArchive.Groups {
					fmt.Println(" •", g)
				}
			}

		case "add-group":
			// add-group <group-name> <path-prefix> [--from-file <file>] <path|dir>...
			if currentArchive == nil {
				sessionErr("no archive open — use 'create' first")
				continue
			}
			if currentWriteFile == nil {
				sessionErr("archive is open for reading, not writing — load after create to add groups")
				continue
			}
			if len(parts) < 4 {
				sessionErr("usage: add-group <group-name> <path-prefix> <path|dir>...")
				continue
			}
			sessionArchiveAddGroup(parts[1:])

		case "close":
			if currentArchive == nil {
				sessionErr("no archive is currently open")
				continue
			}
			if retCode, _ := CloseArchiveGo(); retCode != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Println(" Archive closed and saved.")

		case "extract":
			// extract [--skip <group>]... [--map <group>=<prefix>]...
			if currentArchive == nil {
				sessionErr("no archive open — use 'load' first")
				continue
			}
			if currentReadFile == nil {
				sessionErr("archive is open for writing — close it first, then load to extract")
				continue
			}
			mapping := buildPrefixMapping(currentArchive.Groups, parts[1:])
			if ReadArchiveGo(mapping, func(filesProcessed int64, filesWritten int64, bytesWritten uint64) {}) != rcOK {
				sessionErr(errorMsg)
				continue
			}
			fmt.Println(" Extraction complete.")

		case "list-files":
			if currentArchive == nil {
				sessionErr("no archive open")
				continue
			}
			if len(currentArchive.Files) == 0 {
				fmt.Println(" (no files)")
			} else {
				for _, f := range currentArchive.Files {
					fmt.Printf(" [%s] %s (%s)\n", f.GroupName, f.RelativeFilePath, formatBytes(f.FileLength))
				}
			}

		default:
			sessionErr(fmt.Sprintf("unknown command %q — type 'help'", parts[0]))
		}
	}

	// Ctrl-D: save cleanly if still open
	if currentArchive != nil {
		if retCode, _ := CloseArchiveGo(); retCode != rcOK {
			_, _ = fmt.Fprintf(os.Stderr, "warning: close archive: %s\n", errorMsg)
		}
	}
	fmt.Println("\nSession saved.")
}

// sessionArchiveAddGroup handles: add-group <group-name> <path-prefix> [--from-file <file>] <path|dir>...
func sessionArchiveAddGroup(args []string) {
	if len(args) < 2 {
		sessionErr("usage: add-group <group-name> <path-prefix> [--from-file <file>] <path|dir>...")
		return
	}

	groupName := args[0]
	pathPrefix := args[1]
	rest := args[2:]

	var inputs []string
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--from-file" {
			if i+1 >= len(rest) {
				sessionErr("--from-file requires a path")
				return
			}
			i++
			lines, err := readLines(rest[i])
			if err != nil {
				sessionErr(err.Error())
				return
			}
			inputs = append(inputs, lines...)
		} else {
			inputs = append(inputs, rest[i])
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

	if AddNewGroupGo(groupName, pathPrefix, files,
		func(filesProcessed int64, filesWritten int64, bytesWritten uint64) {},
	) != rcOK {
		sessionErr(errorMsg)
		return
	}

	fmt.Printf(" Group %q added (%d file(s)).\n", groupName, len(files))
}

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

// buildPrefixMapping constructs the map[string]*string for ReadArchiveGo from
// CLI flags. All known groups default to their name as prefix unless overridden
// with --map group=prefix or skipped with --skip group.
//
// Flags:
//
//	--skip <group>          map group -> nil (skip during extraction)
//	--map <group>=<prefix>  map group -> prefix
//
// Groups not mentioned in flags get mapped to their own name as prefix,
// which extracts files relative to the current directory under the group name.
func buildPrefixMapping(groups []string, args []string) map[string]*string {
	mapping := make(map[string]*string, len(groups))

	// default: map each group to itself as prefix
	for _, g := range groups {
		name := g
		mapping[g] = &name
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--skip":
			if i+1 >= len(args) {
				fatalf("--skip requires a group name\n")
			}
			i++
			mapping[args[i]] = nil

		case "--map":
			if i+1 >= len(args) {
				fatalf("--map requires a group=prefix argument\n")
			}
			i++
			eq := strings.IndexByte(args[i], '=')
			if eq < 0 {
				fatalf("--map argument must be in the form group=prefix, got %q\n", args[i])
			}
			group := args[i][:eq]
			prefix := args[i][eq+1:]
			mapping[group] = &prefix
		}
	}

	return mapping
}

func archiveSessionPrompt() string {
	name := "-"
	if currentArchive != nil {
		name = currentArchive.ArchiveName
	}
	return fmt.Sprintf("blobber-archive [%s]> ", name)
}

func printArchiveStatus(folder string) {
	fmt.Println(" Folder:", folder)
	if currentArchive == nil {
		fmt.Println(" Archive: (none)")
		return
	}
	fmt.Println(" Archive:", currentArchive.ArchiveName)
	fmt.Printf(" Groups:  %d\n", len(currentArchive.Groups))
	fmt.Printf(" Files:   %d\n", len(currentArchive.Files))
	if currentWriteFile != nil {
		fmt.Println(" Mode:    write")
	} else if currentReadFile != nil {
		fmt.Println(" Mode:    read")
	}
}

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
		_, _ = fmt.Fprintf(os.Stderr, "warning: close overview: %s\n", errorMsg)
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
	_, _ = fmt.Fprintf(os.Stderr, "error: "+format, a...)
	os.Exit(1)
}

func sessionErr(msg string) {
	_, _ = fmt.Fprintf(os.Stderr, "  error: %s\n", msg)
}

// -----------------------------------------------------------------------------
// HELP TEXT
// -----------------------------------------------------------------------------

func printUsage() {
	fmt.Print(`blobber — blob archive tool

Usage:
  blobber compress   <blob-file> [--level N] <path|dir>...
  blobber decompress <blob-file> <target-path> <position> <length>
  blobber repo    <store-dir> <sub-command> [args...]
  blobber version <store-dir> [<repo-name>]
  blobber archive         <folder> <sub-command> [args...]
  blobber archive-session <folder>

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
 
Archive commands (non-interactive):
  archive <folder> load
      Print the list of groups stored in the archive.
 
  archive <folder> extract [--skip <group>]... [--map <group>=<prefix>]...
      Extract all files from the archive.
      By default each group is extracted under a subdirectory named after the group.
      --skip <group>          Skip this group entirely during extraction.
      --map <group>=<prefix>  Extract this group's files under the given prefix instead.

  IMPORTANT INFORMATION:
      Creation of archives is  only possible in interactive mode.
 
Archive session (interactive):
 
  archive-session <folder>
      Start an interactive archive session for the given folder.
      The archive stays open between commands so you can create an archive,
      add groups, and extract without reloading state on every call.
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

func printArchiveSessionHelp() {
	fmt.Print(`
Archive session commands:
 
  status                   Show current folder, archive name, group and file counts.
  create <name> <creator>  Create a new archive (opens it for writing).
  load                     Load the existing archive in the session folder (opens for reading).
  close                    Save the archive to disk and close it.
  groups                   List all groups in the open archive.
  list-files               List all files recorded in the open archive.
 
  add-group <group-name> <path-prefix> [--from-file <list.txt>] <path|dir>...
      Compress files into the open archive under the given group name.
      path-prefix is stripped from each file path to form the stored relative path.
      --from-file reads newline-separated paths from a text file.
      Directories are walked recursively.
 
  extract [--skip <group>]... [--map <group>=<prefix>]...
      Decompress files from the open archive.
      By default each group is extracted under a subdirectory named after the group.
      --skip <group>         Skip this group entirely.
      --map <group>=<prefix> Extract this group's files under the given prefix.
 
  exit / quit              Save the archive if open and exit the session.
`)
}
