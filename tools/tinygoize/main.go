// Copyright 2017-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// usage: invoke this with a list of directories. For each directory, it will
// run `GOARCH=amd64 GOOS=linux tinygo build -tags tinygo.enable`
// then attempt to fix-up the build tags by either adding or removing an
// go:build expression `(!tinygo || tinygo.enable)`

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"runtime"
)

// Track set of passing, failing, and excluded commands
type BuildStatus struct {
	passing  []string
	failing  []string
	excluded []string
}

// return trimmed output of "tinygo version"
func tinygoVersion(tinygo *string) (string, error) {
	out, err := exec.Command(*tinygo, "version").CombinedOutput()
	if nil != err {
		return "", err
	}
	return strings.TrimSpace(string(out)), err
}

type BuildResult struct {
	dir      string
	code     BuildCode
	err      error
}

func worker(tinygo *string, tasks <-chan string, results chan<- BuildResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for dir := range tasks {
		code, err := build(tinygo, dir)
		// Send the result back to the main routine
		results <- BuildResult {
			dir:      dir,
			code:      code,
			err:      err,
		}
	}
}

// "tinygo build" in each of directories 'dirs'
func buildDirs(tinygo *string, dirs []string) (status BuildStatus, err error) {
	nproc := runtime.NumCPU()
	tasks := make(chan string)
	results := make(chan BuildResult)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < nproc; i++ {
		wg.Add(1)
		go worker(tinygo, tasks, results, &wg)
	}

	// Assign tasks
	go func() {
		for _, dir := range dirs {
			tasks <- dir
		}
		close(tasks) // close channel signals workers to exit when done
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results) // close results channel after all workers done
	}()

	for result := range results {
		if result.err != nil {
			break
		}
		switch result.code {
			case BuildCodeExclude:
				status.excluded = append(status.excluded, result.dir)
			case BuildCodeFailed:
				status.failing = append(status.failing, result.dir)
			case BuildCodeSuccess:
				status.passing = append(status.passing, result.dir)
		}
	}
	return
}

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

func main() {
	pathMD := flag.String("o", "-", "Output file for markdown summary, '-' or '' for STDOUT")
	tinygo := flag.String("tinygo", "tinygo", "Path to tinygo")

	flag.Parse()

	tgVersion, err := tinygoVersion(tinygo)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s\n", tgVersion)

	file := os.Stdout
	if len(*pathMD) > 0 && *pathMD != "-" {
		file, err = os.Create(*pathMD)
		if err != nil {
			fmt.Printf("Error creating opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
	}

	// generate list of commands that pass / fail / are excluded
	status, err := buildDirs(tinygo, flag.Args())
	if nil != err {
		log.Fatal(err)
	}

	// fix-up constraints in failing files
	for _, f := range status.failing {
		err = fixupConstraints(f, false)
		if nil != err {
			log.Fatal(err)
		}
	}

	// fix-up constraints in passing files
	for _, f := range status.passing {
		err = fixupConstraints(f, true)
		if nil != err {
			log.Fatal(err)
		}
	}

	// write markdown output
	err = writeMarkdown(file, pathMD, &tgVersion, status)
	if nil != err {
		log.Fatal(err)
	}
}
