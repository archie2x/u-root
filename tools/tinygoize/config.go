// Copyright 2017-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

type Config struct {
	pathMD string
	tinygo string
	nWorkers int
	checkOnly bool
	verbose bool
	dirs []string
}
