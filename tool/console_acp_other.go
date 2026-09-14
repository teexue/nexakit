//go:build !windows

package tool

func hostACP() uint32 { return 65001 }
