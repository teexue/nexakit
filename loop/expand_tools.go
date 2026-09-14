package loop

import "github.com/teexue/nexakit/tool"

// ExpandImpliedTools adds tools that share capability with an already-selected
// tool. read_image is implied by read_file so agents need not list it separately.
func ExpandImpliedTools(names []string) []string {
	hasReadFile, hasReadImage := false, false
	for _, n := range names {
		switch n {
		case tool.ReadFileName:
			hasReadFile = true
		case tool.ReadImageName:
			hasReadImage = true
		}
	}
	if !hasReadFile || hasReadImage {
		return names
	}
	out := make([]string, len(names), len(names)+1)
	copy(out, names)
	return append(out, tool.ReadImageName)
}
