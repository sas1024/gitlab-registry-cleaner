package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/xanzy/go-gitlab"
)

type Config struct {
	GitlabBaseUrl      string
	AuthToken          string
	ExcludedTags       []string
	CheckCommitsBranch string
	CheckCommitsCount  int
}

var (
	flConfigPath = flag.String("config", "config.toml", "Path to config file")
	cfg          Config
)

func main() {
	flag.Parse()

	_, err := toml.DecodeFile(*flConfigPath, &cfg)
	if err != nil {
		die(err)
	}

	git := gitlab.NewClient(nil, cfg.AuthToken)
	err = git.SetBaseURL(cfg.GitlabBaseUrl)
	if err != nil {
		die(err)
	}

	pTrue := true
	opts := &gitlab.ListProjectsOptions{Membership: &pTrue}
	projects, _, err := git.Projects.ListProjects(opts)
	if err != nil {
		die(err)
	}

	for _, p := range projects {

		repositories, _, err := git.ContainerRegistry.ListRegistryRepositories(p.ID, nil)
		if err != nil {
			log.Println(fmt.Sprintf("Skip \"%s\" repository. Error: %v", p.Name, err))
			continue
		}

		commits, _, err := git.Commits.ListCommits(p.ID, &gitlab.ListCommitsOptions{RefName: &cfg.CheckCommitsBranch})

		var filteredCommits []string

		if len(commits) > cfg.CheckCommitsCount {
			for _, c := range commits[:cfg.CheckCommitsCount] {
				filteredCommits = append(filteredCommits, c.ShortID)
			}
		}

		for _, r := range repositories {
			tags, _, err := git.ContainerRegistry.ListRegistryRepositoryTags(p.ID, r.ID, nil)
			if err != nil {
				die(err)
			}

			filtered := filter(tags, filteredCommits)
			filtered = filter(filtered, cfg.ExcludedTags)

			for _, t := range filtered {
				log.Println(fmt.Sprintf("Delete %s", t.Location))
				_, err := git.ContainerRegistry.DeleteRegistryRepositoryTag(p.ID, r.ID, t.Name)
				if err != nil {
					die(err)
				}

			}
		}
	}
}

func filter(source []*gitlab.RegistryRepositoryTag, filter []string) (filtered []*gitlab.RegistryRepositoryTag) {
	for _, t := range source {
		found := false
		for _, c := range filter {
			if t.Name == c {
				found = true
			}
		}
		if !found {
			filtered = append(filtered, t)
		}
	}
	return
}

// die calls log.Fatal if err wasn't nil.
func die(err error) {
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Fatal(err)
	}
}
