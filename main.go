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

		commits, _, err := git.Commits.ListCommits(p.ID, &gitlab.ListCommitsOptions{ListOptions: gitlab.ListOptions{PerPage: 200}, RefName: &cfg.CheckCommitsBranch, All: &pTrue})

		for _, r := range repositories {
			tags, _, err := git.ContainerRegistry.ListRegistryRepositoryTags(p.ID, r.ID, nil)
			if err != nil {
				die(err)
			}

			deleteTags := filter(tags, cfg.ExcludedTags)

			cc := commitsFromTags(tags, commits)
			var filteredCommits []string
			if len(cc) >= cfg.CheckCommitsCount {
				for _, c := range cc[:cfg.CheckCommitsCount] {
					filteredCommits = append(filteredCommits, c)
				}
			} else {
				filteredCommits = cc
			}

			deleteTags = filter(deleteTags, filteredCommits)

			for _, t := range deleteTags {
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
		for _, f := range filter {
			if t.Name == f {
				found = true
			}
		}
		if !found {
			filtered = append(filtered, t)
		}
	}
	return
}

func commitsFromTags(tags []*gitlab.RegistryRepositoryTag, commits []*gitlab.Commit) (filtered []string) {
	for _, t := range tags {
		for _, c := range commits {
			if t.Name != c.ShortID {
				continue
			}
			filtered = append(filtered, c.ShortID)
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
