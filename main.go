package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

// MaxConcurrentClones is the upper limit of the maximum number of
// concurrent git clones
const MaxConcurrentClones = 20

const defaultMaxUserMigrationRetry = 5

var gitHostToken string
var useHTTPSClone *bool
var ignorePrivate *bool
var gitHostUsername string

// The services we know of and their default public host names
var knownServices = map[string]string{
	"github":    "github.com",
	"gitlab":    "gitlab.com",
	"bitbucket": "bitbucket.org",
	"forgejo":   "codeberg.org",
}

// parseSubcommandFlags parses --config and --help flags for a subcommand.
func parseSubcommandFlags(name, description string, args []string) string {
	var configPath string
	fs := flag.NewFlagSet("gitbackup "+name, flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file (default: OS config directory)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: gitbackup %s [--config path]\n\n", name)
		fmt.Fprintf(os.Stderr, "%s\n\n", description)
		fs.PrintDefaults()
	}
	fs.Parse(args)
	return configPath
}

func main() {

	// Handle subcommands before flag parsing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			configPath := parseSubcommandFlags("init", "Create a default gitbackup.yml configuration file.", os.Args[2:])
			if err := handleInitConfig(configPath); err != nil {
				log.Fatal(err)
			}
			return
		case "validate":
			configPath := parseSubcommandFlags("validate", "Validate the gitbackup.yml configuration file.", os.Args[2:])
			if err := handleValidateConfig(configPath); err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	c, err := initConfig(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	err = validateConfig(c)
	if err != nil {
		log.Fatal(err)
	}

	client := newClient(c.service, c.gitHostURL)
	var executionErr error

	// TODO implement validation of options so that we don't
	// allow multiple operations at one go
	if c.githubListUserMigrations {
		handleGithubListUserMigrations(client, c)
	} else if c.githubCreateUserMigration {
		handleGithubCreateUserMigration(client, c)
	} else {
		executionErr = handleGitRepositoryClone(client, c)
	}
	if executionErr != nil {
		log.Fatal(executionErr)
	}
}
