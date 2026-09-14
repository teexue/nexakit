package event_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrintEventsSwitchCoversAllTypes(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(file), "event.go"))
	require.NoError(t, err)
	body := braceBody(string(src), "func PrintEvents")
	require.NotEmpty(t, body)
	for _, name := range typeConstNames(string(src)) {
		require.Contains(t, body, name)
	}
}

func typeConstNames(src string) []string {
	re := regexp.MustCompile(`(Type\w+)\s+Type = "`)
	var names []string
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		names = append(names, m[1])
	}
	return names
}

func braceBody(src, sig string) string {
	i := strings.Index(src, sig)
	if i < 0 {
		return ""
	}
	open := strings.Index(src[i:], "{")
	if open < 0 {
		return ""
	}
	start := i + open
	depth := 0
	for j := start; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : j+1]
			}
		}
	}
	return ""
}
