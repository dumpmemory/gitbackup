package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultConfigFile = "gitbackup.yml"

// defaultConfigPath returns the OS-specific default path for the config file.
// On Linux: ~/.config/gitbackup/gitbackup.yml
// On macOS: ~/Library/Application Support/gitbackup/gitbackup.yml
// On Windows: %AppData%/gitbackup/gitbackup.yml
func defaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine config directory: %v", err)
	}
	return filepath.Join(configDir, "gitbackup", defaultConfigFile), nil
}

// resolveConfigPath returns the config path to use.
// If configPath is non-empty, it is returned as-is.
// Otherwise, the OS-specific default path is returned.
func resolveConfigPath(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	return defaultConfigPath()
}

// fileConfig represents the YAML configuration file structure.
// Migration-related flags are intentionally excluded as they
// are one-off operations better suited to CLI flags.
type fileConfig struct {
	Service       string       `yaml:"service"`
	GitHostURL    string       `yaml:"githost_url"`
	BackupDir     string       `yaml:"backup_dir"`
	IgnorePrivate bool         `yaml:"ignore_private"`
	IgnoreFork    bool         `yaml:"ignore_fork"`
	UseHTTPSClone bool         `yaml:"use_https_clone"`
	Bare          bool         `yaml:"bare"`
	GitHub        githubConfig `yaml:"github"`
	GitLab        gitlabConfig `yaml:"gitlab"`
	Forgejo       forgejoConfig `yaml:"forgejo"`
}

type githubConfig struct {
	RepoType           string   `yaml:"repo_type"`
	NamespaceWhitelist []string `yaml:"namespace_whitelist"`
}

type gitlabConfig struct {
	ProjectVisibility     string `yaml:"project_visibility"`
	ProjectMembershipType string `yaml:"project_membership_type"`
}

type forgejoConfig struct {
	RepoType string `yaml:"repo_type"`
}

// defaultFileConfig returns a fileConfig with the same defaults as the CLI flags
func defaultFileConfig() fileConfig {
	return fileConfig{
		Service:       "github",
		GitHostURL:    "",
		BackupDir:     "",
		IgnorePrivate: false,
		IgnoreFork:    false,
		UseHTTPSClone: false,
		Bare:          false,
		GitHub: githubConfig{
			RepoType:           "all",
			NamespaceWhitelist: []string{},
		},
		GitLab: gitlabConfig{
			ProjectVisibility:     "internal",
			ProjectMembershipType: "all",
		},
		Forgejo: forgejoConfig{
			RepoType: "user",
		},
	}
}

// handleInitConfig creates a default gitbackup.yml at the given path,
// or at the OS-specific default location if configPath is empty.
func handleInitConfig(configPath string) error {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory %s: %v", dir, err)
	}

	cfg := defaultFileConfig()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("error generating config: %v", err)
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("error writing %s: %v", path, err)
	}

	fmt.Printf("Created %s\n", path)
	return nil
}

// fileConfigToAppConfig converts a fileConfig into an appConfig.
// Migration-related fields are left at their zero values since they
// are CLI-only flags.
func fileConfigToAppConfig(fc *fileConfig) *appConfig {
	return &appConfig{
		service:                     fc.Service,
		gitHostURL:                  fc.GitHostURL,
		backupDir:                   fc.BackupDir,
		ignorePrivate:               fc.IgnorePrivate,
		ignoreFork:                  fc.IgnoreFork,
		useHTTPSClone:               fc.UseHTTPSClone,
		bare:                        fc.Bare,
		githubRepoType:              fc.GitHub.RepoType,
		githubNamespaceWhitelist:    fc.GitHub.NamespaceWhitelist,
		gitlabProjectVisibility:     fc.GitLab.ProjectVisibility,
		gitlabProjectMembershipType: fc.GitLab.ProjectMembershipType,
		forgejoRepoType:             fc.Forgejo.RepoType,
	}
}

// loadConfigFile reads and parses the config file at the given path,
// or at the OS-specific default location if configPath is empty.
func loadConfigFile(configPath string) (*fileConfig, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", path, err)
	}

	var cfg fileConfig
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", path, err)
	}
	return &cfg, nil
}

// handleValidateConfig reads the config file and validates its contents.
// If configPath is empty, the OS-specific default location is used.
func handleValidateConfig(configPath string) error {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}

	cfg, err := loadConfigFile(configPath)
	if err != nil {
		return err
	}

	var errors []string

	// Validate service
	if _, ok := knownServices[cfg.Service]; !ok {
		errors = append(errors, fmt.Sprintf("invalid service: %q (must be github, gitlab, bitbucket, or forgejo)", cfg.Service))
	}

	// Validate service-specific field values
	switch cfg.Service {
	case "github":
		if !contains([]string{"all", "owner", "member", "starred"}, cfg.GitHub.RepoType) {
			errors = append(errors, fmt.Sprintf("invalid github.repo_type: %q (must be all, owner, member, or starred)", cfg.GitHub.RepoType))
		}
	case "gitlab":
		if !contains([]string{"internal", "public", "private"}, cfg.GitLab.ProjectVisibility) {
			errors = append(errors, fmt.Sprintf("invalid gitlab.project_visibility: %q (must be internal, public, or private)", cfg.GitLab.ProjectVisibility))
		}
		if !validGitlabProjectMembership(cfg.GitLab.ProjectMembershipType) {
			errors = append(errors, fmt.Sprintf("invalid gitlab.project_membership_type: %q (must be all, owner, member, or starred)", cfg.GitLab.ProjectMembershipType))
		}
	case "forgejo":
		if !contains([]string{"user", "starred"}, cfg.Forgejo.RepoType) {
			errors = append(errors, fmt.Sprintf("invalid forgejo.repo_type: %q (must be user or starred)", cfg.Forgejo.RepoType))
		}
	}

	// Validate required environment variables
	switch cfg.Service {
	case "github":
		if os.Getenv("GITHUB_TOKEN") == "" {
			errors = append(errors, "GITHUB_TOKEN environment variable not set")
		}
	case "gitlab":
		if os.Getenv("GITLAB_TOKEN") == "" {
			errors = append(errors, "GITLAB_TOKEN environment variable not set")
		}
	case "bitbucket":
		if os.Getenv("BITBUCKET_USERNAME") == "" {
			errors = append(errors, "BITBUCKET_USERNAME environment variable not set")
		}
		if os.Getenv("BITBUCKET_TOKEN") == "" && os.Getenv("BITBUCKET_PASSWORD") == "" {
			errors = append(errors, "BITBUCKET_TOKEN or BITBUCKET_PASSWORD environment variable must be set")
		}
	case "forgejo":
		if os.Getenv("FORGEJO_TOKEN") == "" {
			errors = append(errors, "FORGEJO_TOKEN environment variable not set")
		}
	}

	if len(errors) > 0 {
		fmt.Println("Validation errors:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("config validation failed")
	}

	fmt.Printf("%s is valid\n", path)
	return nil
}
