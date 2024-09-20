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
	"io"
	"os/exec"
	"strings"
	"sync"
	"runtime"
	"golang.org/x/term"
)


// return trimmed output of "tinygo version"
func tinygoVersion(tinygo *string) (string, error) {
	out, err := exec.Command(*tinygo, "version").CombinedOutput()
	if nil != err {
		return "", err
	}
	return strings.TrimSpace(string(out)), err
}

func progress(nComplete int, outOf int) {
	isTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	if isTerminal {
		fmt.Printf("\033[2K\r")
	}
	percent := 100 * ( float64(nComplete) / float64(outOf))
	fmt.Printf("%v/%v %.2f%%", nComplete, outOf, percent)
	if !isTerminal || nComplete == outOf {
		fmt.Println()
	}
}

// Track set of passing, failing, and excluded commands
type BuildStatus struct {
	passing  []string
	failing  []string
	excluded []string
	modified []string
}

type WorkerResult struct {
	dir string
	buildRes BuildRes
	didWork  bool // whether files in the package need(ed) constraint update
	err error
}

func worker(id int, conf *Config, tasks <-chan string, results chan<- WorkerResult, workGroup *sync.WaitGroup) {
	defer workGroup.Done()
	for dir := range tasks {
		br, err := build(id, &conf.tinygo, dir)
		var dw bool
		if err == nil && !br.excluded {
			dw, err = fixupPkgConstraints(dir, br.err == nil, conf.checkOnly)
		}

		// send result back to main routine
		results <- WorkerResult {
			dir: dir,
			buildRes: br,
			didWork: dw,
			err: err,
		}
	}
}

// "tinygo build" in each of directories 'dirs'
func buildDirs(conf *Config) (status BuildStatus, err error) {
	jobs := len(conf.dirs)
	nWorkers := conf.nWorkers
	if conf.nWorkers <= 0 {
		nWorkers = runtime.NumCPU()
	}
	if nWorkers > jobs {
		nWorkers = jobs
	}
	tasks := make(chan string)
	results := make(chan WorkerResult)
	var wg sync.WaitGroup

	// Start workers
	log.Printf("Spawning %v workers", nWorkers)
	for id := 0; id < nWorkers; id++ {
		wg.Add(1)
		go worker(id+1, conf, tasks, results, &wg)
	}

	// Assign tasks
	go func() {
		for _, dir := range conf.dirs {
			tasks <- dir
		}
		close(tasks) // close channel signals workers to exit when done
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results) // close results channel after all workers done
	}()

	nComplete := 0
	for result := range results {
		nComplete += 1
		progress(nComplete, jobs)

		if result.err != nil {
			break
		}
		if result.buildRes.excluded {
			status.excluded = append(status.excluded, result.dir)
		} else if result.buildRes.err != nil {
			status.failing = append(status.failing, result.dir)
		} else {
			status.passing = append(status.passing, result.dir)
		}
		if result.didWork {
			status.modified = append(status.modified, result.dir)
		}
	}
	return
}

func main() {
	conf := Config{}
	flag.StringVar(&conf.pathMD, "o", "-", "Output file for markdown summary, '-' or '' for STDOUT")
	flag.StringVar(&conf.tinygo, "tinygo", "tinygo", "Path to tinygo")
	flag.IntVar(&conf.nWorkers, "j", 0, "Allow 'j' jobs at once; NumCPU() jobs with no arg.")
	flag.BoolVar(&conf.checkOnly, "n", false, "Check-only, do not modify sources")
	flag.BoolVar(&conf.verbose, "v", false, "Verbose")

	flag.Parse()
	conf.dirs = flag.Args()

	if !conf.verbose {
		log.SetOutput(io.Discard)
	}

	tgVersion, err := tinygoVersion(&conf.tinygo)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s\n", tgVersion)

	file := os.Stdout
	if len(conf.pathMD) > 0 && conf.pathMD != "-" {
		file, err = os.Create(conf.pathMD)
		if err != nil {
			fmt.Printf("Error creating opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
	}

	// generate list of commands that pass / fail / are excluded
	status, err := buildDirs(&conf)
	if nil != err {
		log.Fatal(err)
	}

	// fix-up constraints in failing files
	// for _, f := range status.failing {
	// 	dw, err := fixupPkgConstraints(f, false, conf.checkOnly)
	// 	if nil != err {
	// 		log.Fatal(err)
	// 	}
	// 	if dw {
	// 		modified = append(modified, f)
	// 	}
	// }

	// // fix-up constraints in passing files
	// for _, f := range status.passing {
	// 	dw, err := fixupPkgConstraints(f, true, conf.checkOnly)
	// 	if nil != err {
	// 		log.Fatal(err)
	// 	}
	// 	if dw {
	// 		modified = append(modified, f)
	// 	}
	// }

	// write markdown output
	err = writeMarkdown(file, &conf.pathMD, &tgVersion, status)
	if nil != err {
		log.Fatal(err)
	}

	if len(status.modified) > 0 {
		fmt.Println("Updates required in package(s):")
		for _,modded := range status.modified {
			fmt.Println(modded)
		}
		os.Exit(1)
	} else {
		fmt.Println("Build constraints up to date.")
	}
}
