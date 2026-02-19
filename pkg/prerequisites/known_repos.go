package prerequisites

// buildKnownRepo returns prerequisite definitions for all known repos.
func buildKnownRepo() map[string]Repo {
	return map[string]Repo{
		"lab-backend": {
			RepoName: "lab-backend",
			Prerequisites: []Prerequisite{
				{
					Type:            TypeFileCopy,
					Description:     "Copy .env.example to .env",
					SourcePath:      ".env.example",
					DestinationPath: ".env",
					SkipIfExists:    ".env",
				},
			},
		},
		"lab": {
			RepoName: "lab",
			Prerequisites: []Prerequisite{
				{
					Type:         TypeCommand,
					Description:  "Install frontend dependencies",
					Command:      "pnpm",
					Args:         []string{"install"},
					WorkingDir:   ".",
					SkipIfExists: "node_modules",
				},
				{
					Type:         TypeCommand,
					Description:  "Build frontend for bundling",
					Command:      "pnpm",
					Args:         []string{"run", "build"},
					WorkingDir:   ".",
					SkipIfExists: "dist",
				},
			},
		},
		"xatu-cbt": {
			RepoName: "xatu-cbt",
			Prerequisites: []Prerequisite{
				{
					Type:            TypeFileCopy,
					Description:     "Copy example.env to .env",
					SourcePath:      "example.env",
					DestinationPath: ".env",
					SkipIfExists:    ".env",
				},
			},
		},
		"cbt-api": {
			RepoName: "cbt-api",
			Prerequisites: []Prerequisite{
				{
					Type:            TypeFileCopy,
					Description:     "Copy config.example.yaml to config.yaml",
					SourcePath:      "config.example.yaml",
					DestinationPath: "config.yaml",
					SkipIfExists:    "config.yaml",
				},
			},
		},
		// cbt has no prerequisites (yet)
	}
}
