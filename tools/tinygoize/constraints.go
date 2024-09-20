// Copyright 2017-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	goBuild    = "//go:build "
	constraint = "!tinygo || tinygo.enable"
)

// Modifies, adds, or removes //go:build line as appropriate with '!tinygo ||
// tinygo.enable'
func fixupFileConstraints(file string, builds bool, checkonly bool) (mustDoWork bool, err error) {
	log.Printf("Process %s", file)
	b, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, file, string(b), parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		log.Fatalf("parsing\n%v\n:%v", string(b), err)
	}

	goBuildPresent := false

	// modify existing //go:build line
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if !strings.HasPrefix(c.Text, goBuild) {
				continue
			}
			goBuildPresent = true

			contains := strings.Contains(c.Text, constraint)

			if (builds && !contains) || (!builds && contains) {
				log.Printf("Skipped, constraint up-to-date: %s\n", file)
				return
			}

			if builds {
				re := regexp.MustCompile(`\(?\s*!tinygo\s+\|\|\s+tinygo.enable\s*\)?(\s+\&\&)?`)
				c.Text = re.ReplaceAllString(c.Text, "")
				log.Printf("Stripping build constraint %v\n", file)

				// handle potentially now-empty build constraint
				re = regexp.MustCompile(`^\s*//go:build\s*$`)
				if re.MatchString(c.Text) {
					filtered := []*ast.Comment{}
					for _, comment := range cg.List {
						if !re.MatchString(comment.Text) {
							filtered = append(filtered, comment)
						}
					}
					cg.List = filtered
				}
			} else {
				c.Text = goBuild + "(" + constraint + ") && (" + c.Text[len(goBuild):] + ")"
			}
			break
		}
	}

	// if it doesn't build but no //go:build found, insert one XXX skip space after copyright
	if !builds && !goBuildPresent {
		// no //go:build line found: insert one
		var cg ast.CommentGroup
		cg.List = append(cg.List, &ast.Comment{Text: goBuild + constraint})

		if len(f.Comments) > 0 {
			// insert //go:build after first comment
			// group, assumed copyright. Doesn't seem
			// quite right but seems to work.
			cg.List[0].Slash = f.Comments[0].List[0].Slash + 1
			f.Comments = append([]*ast.CommentGroup{f.Comments[0], &cg}, f.Comments[1:]...)
		} else {
			// prepend //go:build
			f.Comments = append([]*ast.CommentGroup{&cg}, f.Comments...)
		}
	}

	// Complete source file.
	var buf bytes.Buffer
	p := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err = p.Fprint(&buf, fset, f); err != nil {
		log.Fatalf("Printing:%v", err)
	}


	if bytes.Equal(b, buf.Bytes()) {
		log.Printf("Skipped, constraint up-to-date: %s\n", file)
		return
	} else {
		mustDoWork = true
	}
	if checkonly {
		return
	}

	if err := os.WriteFile(file, buf.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	return
}

// fixup build constraint lines for all .go files in pkg
func fixupPkgConstraints(dir string, builds bool, checkonly bool) (mustDoWork bool, err error) {
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		log.Fatal(err)
	}
	for _, file := range files {
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		mdw, err2 := fixupFileConstraints(file, builds, checkonly)
		if err2 != nil {
			err = err2
			return
		}
		if mdw {
			mustDoWork = true
		}
	}
	return
}
