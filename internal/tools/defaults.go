package tools

// Defaults returns the tools DevDoctor knows how to check in Phase 2.
// This list is policy, not mechanism: adding a tool is one entry here
// and no changes anywhere else.
func Defaults() []Tool {
	return []Tool{
		{
			Name:           "Git",
			Command:        "git",
			VersionArgs:    []string{"--version"},
			VersionPattern: `git version (\S+)`,
		},
		{
			Name:           "Node.js",
			Command:        "node",
			VersionArgs:    []string{"--version"},
			VersionPattern: `v?(\d[\w.+-]*)`,
		},
		{
			Name:           "npm",
			Command:        "npm",
			VersionArgs:    []string{"--version"},
			VersionPattern: `(\d[\w.+-]*)`,
		},
		{
			Name:           "Python",
			Command:        "python",
			VersionArgs:    []string{"--version"},
			VersionPattern: `Python (\d[\w.+-]*)`,
		},
		{
			Name:           "pip",
			Command:        "pip",
			VersionArgs:    []string{"--version"},
			VersionPattern: `pip (\d[\w.+-]*)`,
		},
		{
			Name:           "Docker",
			Command:        "docker",
			VersionArgs:    []string{"--version"},
			VersionPattern: `Docker version (\d[\d.]*)`,
		},
	}
}
