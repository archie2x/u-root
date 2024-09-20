package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"os"
)

func writeMarkdown(file *os.File, pathMD *string, tgVersion *string, status BuildStatus) (err error) {
	// (not string literal because conflict with markdown back-tick)
	fmt.Fprintf(file, "---\n\n")
	fmt.Fprintf(file, "DO NOT EDIT.\n\n")
	fmt.Fprintf(file, "Generated via `go run tools/tinygoize/main.go`\n\n")
	fmt.Fprintf(file, "%v\n\n", *tgVersion)
	fmt.Fprintf(file, "---\n\n")

	fmt.Fprintf(file, `# Status of u-root + tinygo
This document aims to track the process of enabling all u-root commands
to be built using tinygo. It will be updated as more commands can be built via:

    u-root> go run tools/tinygoize/* cmds/{core,exp,extra}/*

Commands that cannot be built with tinygo have a \"(!tinygo || tinygo.enable)\"
build constraint. Specify the "tinygo.enable" build tag to attempt to build
them.

    tinygo build -tags tinygo.enable cmds/core/ls

The list below is the result of building each command for Linux, x86_64.

The necessary additions to tinygo will be tracked in
[#2979](https://github.com/u-root/u-root/issues/2979).

---

## Commands Build Status
`)

	linkText := func(dir string) string {
		// ignoring err here because pathMD already opened(exists) and
		// dir already checked
		relPath, _ := filepath.Rel(filepath.Dir(*pathMD), dir)
		return fmt.Sprintf("[%v](%v)", dir, relPath)
	}

	processSet := func(header string, dirs []string) {
		fmt.Fprintf(file, "\n### %v (%v commands)\n", header, len(dirs))
		sort.Strings(dirs)

		if len(dirs) == 0 {
			fmt.Fprintf(file, "NONE\n")
		}
		for _, dir := range dirs {
			msg := fmt.Sprintf(" - %v", linkText(dir))
			tags := buildTags(dir)
			if len(tags) > 0 {
				msg += fmt.Sprintf(" tags: %v", tags)
			}
			fmt.Fprintf(file, "%v\n", msg)
		}
	}

	processSet("EXCLUDED", status.excluded)
	processSet("FAILING", status.failing)
	processSet("PASSING", status.passing)

	return
}

