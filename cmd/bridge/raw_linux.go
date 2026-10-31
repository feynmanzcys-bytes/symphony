//go:build linux

package main

import "syscall"

const termiosGet, termiosSet = syscall.TCGETS, syscall.TCSETS
