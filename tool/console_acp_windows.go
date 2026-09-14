//go:build windows

package tool

import "golang.org/x/sys/windows"

func hostACP() uint32 { return windows.GetACP() }
