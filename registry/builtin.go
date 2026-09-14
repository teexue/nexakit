package registry

import (
	"github.com/teexue/nexakit/subagent"
	"github.com/teexue/nexakit/tool"
)

// RegisterBuiltin registers every built-in tool into r.
// workDir is the sandbox root for file operation tools (typically the agent's
// home directory).
func RegisterBuiltin(r *Registry, workDir string) {
	r.MustRegister(tool.GetTime{})

	// File operation tools
	r.MustRegister(tool.ReadFile{WorkDir: workDir})
	r.MustRegister(tool.ReadImage{WorkDir: workDir})
	r.MustRegister(tool.WriteFile{WorkDir: workDir})
	r.MustRegister(tool.ListDirectory{WorkDir: workDir})
	r.MustRegister(tool.EditFile{WorkDir: workDir})
	r.MustRegister(tool.CreateDirectory{WorkDir: workDir})
	r.MustRegister(tool.DeleteFile{WorkDir: workDir})
	r.MustRegister(tool.SearchFiles{WorkDir: workDir})

	// Command execution
	r.MustRegister(tool.RunCommand{WorkDir: workDir})

	// Network
	r.MustRegister(tool.WebFetch{})

	// Sub-agent delegation (registered by name; the tool itself lives in the
	// subagent package where the nested-run wiring is defined).
	r.MustRegister(subagent.DelegateTask{})
}
