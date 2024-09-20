// Copyright 2017-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"os"
	"os/exec"
	"strings"
)

type BuildCode int

const (
	BuildCodeSuccess BuildCode = iota
	BuildCodeFailed
	BuildCodeExclude
	BuildCodeFatal
)

type BuildRes struct {
	err *exec.ExitError
	excluded bool
	output []byte
}

// Additional tags required for specific commands. Assumes command names are
// unique despite being in different directories.
var addBuildTags = map[string]string{
	"gzip":     "noasm",
	"insmod":   "noasm",
	"rmmod":    "noasm",
	"bzimage":  "noasm",
	"kconf":    "noasm",
	"modprobe": "noasm",
	"console":  "noasm",
	"init":     "noasm",
}

// Returns the needed build-tags for a given package
func buildTags(dir string) (tags string) {
	parts := strings.Split(dir, "/")
	cmd := parts[len(parts)-1]
	return addBuildTags[cmd]
}

// check (via `go build -n`) if a given directory would have been skipped
// due to build constraints (e.g. cmds/core/bind only builds for plan9)
func isExcluded(dir string) bool {
	// too lazy to dynamically pull tags from `tinygo info`
	tags := []string{
		"tinygo",
		"tinygo.enable",
		"purego",
		"osusergo",
		"math_big_pure_go",
		"gc.precise",
		"scheduler.tasks",
		"serial.none",
	}
	c := exec.Command("go", "build",
		"-n",
		"-tags", strings.Join(tags, ","),
	)
	c.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	c.Dir = dir
	out, _ := c.CombinedOutput()
	return strings.Contains(string(out), "build constraints exclude all Go files in")
}

// "tinygo build" in directory 'dir'
func build(id int, tinygo *string, dir string) (res BuildRes, err error) {
	wlog := func(format string, args ...interface{}) {
		log.Printf("[%d] "+format, append([]interface{}{id}, args...)...)
	}
	wlog("%s Building...\n", dir)
	tags := []string{"tinygo.enable"}
	if addTags := buildTags(dir); addTags != "" {
		tags = append(tags, addTags)
	}
	cmd := exec.Command(*tinygo, "build", "-tags", strings.Join(tags, ","))
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	cmd.Dir = dir
	res.output, err = cmd.CombinedOutput()
	if err != nil {
		var ok bool
		res.err, ok = err.(*exec.ExitError)
		if !ok {
			return
		}
		err = nil
		if isExcluded(dir) {
			wlog("%v EXCLUDED\n", dir)
			res.excluded = true
			return
		}
		lines := strings.Split(string(res.output), "\n")
		for _,line := range lines {
			wlog(line)
		}
		wlog("%v FAILED %v\n", dir, res.err)
		return
	}
	wlog("%v PASS\n", dir)
	return
}
