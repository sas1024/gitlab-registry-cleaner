package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/xanzy/go-gitlab"
)

type Config struct {
	GitlabBaseUrl     string
	AuthToken         string
	ExcludedTags      []string
	SaveRevisionCount int
}

var (
	flConfigPath = flag.String("config", "config.toml", "Path to config file")
	cfg          Config
)

type TagsList []*gitlab.RegistryRepositoryTag

func (t TagsList) Len() int           { return len(t) }
func (t TagsList) Less(i, j int) bool { return t[i].CreatedAt.Before(*t[j].CreatedAt) }
func (t TagsList) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }

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

		if p.Name != "desktop" {
			continue
		}

		repositories, _, err := git.ContainerRegistry.ListRegistryRepositories(p.ID, nil)
		if err != nil {
			log.Println(fmt.Sprintf("Skip \"%s\" repository. Error: %v", p.Name, err))
			continue
		}

		for _, r := range repositories {
			tt, _, err := git.ContainerRegistry.ListRegistryRepositoryTags(p.ID, r.ID, nil)
			if err != nil {
				die(err)
			}

			var tags []*gitlab.RegistryRepositoryTag
			for _, t := range tt {
				tag, _, err := git.ContainerRegistry.GetRegistryRepositoryTagDetail(p.ID, r.ID, t.Name)
				if err != nil {
					die(err)
				}
				tags = append(tags, tag)
			}

			excludedRevisions := make(map[string]bool)

			for _, t := range tags {
				for _, e := range cfg.ExcludedTags {
					if e != t.Name {
						continue
					}
					excludedRevisions[t.ShortRevision] = true
				}
			}

			tags = filter(tags, cfg.ExcludedTags)

			var filteredTags TagsList
			for _, t := range tags {
				if _, ok := excludedRevisions[t.ShortRevision]; !ok {
					filteredTags = append(filteredTags, t)
				}
			}

			sort.Sort(filteredTags)

			if len(filteredTags) <= cfg.SaveRevisionCount {
				// skip registry tags deletion
				continue
			}

			var deleteTags TagsList
			for _, t := range filteredTags[:len(filteredTags)-cfg.SaveRevisionCount] {
				deleteTags = append(deleteTags, t)
			}

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

// die calls log.Fatal if err wasn't nil.
func die(err error) {
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Fatal(err)
	}
}
