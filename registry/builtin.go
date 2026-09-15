package registry

import (
	"github.com/teexue/nexakit/subagent"
	"github.com/teexue/nexakit/tool/builtin"
)

// RegisterBuiltin registers every built-in tool into r.
// workDir is the sandbox root for file operation tools (typically the agent's
// home directory).
func RegisterBuiltin(r *Registry, workDir string) {
	r.MustRegister(builtin.GetTime{})

	// File operation tools
	r.MustRegister(builtin.ReadFile{WorkDir: workDir})
	r.MustRegister(builtin.ReadImage{WorkDir: workDir})
	r.MustRegister(builtin.WriteFile{WorkDir: workDir})
	r.MustRegister(builtin.ListDirectory{WorkDir: workDir})
	r.MustRegister(builtin.EditFile{WorkDir: workDir})
	r.MustRegister(builtin.CreateDirectory{WorkDir: workDir})
	r.MustRegister(builtin.DeleteFile{WorkDir: workDir})
	r.MustRegister(builtin.SearchFiles{WorkDir: workDir})

	// Command execution
	r.MustRegister(builtin.RunCommand{WorkDir: workDir})

	// Network
	r.MustRegister(builtin.WebFetch{})

	// Sub-agent delegation (registered by name; the tool itself lives in the
	// subagent package where the nested-run wiring is defined).
	r.MustRegister(subagent.DelegateTask{})
}
