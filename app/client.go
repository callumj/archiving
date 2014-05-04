package app

import (
	"callumj.com/weave/core"
	"log"
	"os"
	"regexp"
	"strings"
)

func performExtraction(args []string) {
	if len(args) < 3 {
		log.Printf("Usage: %v ENCRYPTED_FILE KEY_FILE [OUT_DIRECTORY]\r\n", args[0])
		panicQuit()
	}

	target := args[1]
	keyfile := args[2]

	var out string

	var success bool
	if len(args) >= 4 {
		out = strings.Join([]string{args[3], "tmp"}, ".")
		success = core.DecryptFile(target, out, keyfile)
	} else {
		out = strings.Replace(target, ".enc", "", 1)
		out = strings.Join([]string{out, "tmp"}, ".")
		if out == target {
			log.Println("Cannot determine the out file, please specify")
			panicQuit()
		}
		success = core.DecryptFile(target, out, keyfile)
	}

	var ensureDirectory = regexp.MustCompile(`(\.(tmp|tgz|tar|gz))+`)
	directory := ensureDirectory.ReplaceAllString(out, "")

	if !core.PathExists(directory) {
		os.Mkdir(directory, 0770)
	}

	if success {
		core.ExtractArchive(out, directory)
	}

	core.CleanUpIfNeeded(out)

	if !success {
		panicQuit()
	}
}