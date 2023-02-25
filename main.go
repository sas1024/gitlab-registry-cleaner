package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/xanzy/go-gitlab"
)

type Config struct {
	GitlabBaseUrl       string
	AuthToken           string
	ExcludedTags        []string
	ReviewAppsTags      []string
	SaveRevisionCount   int
	SaveReviewAppsCount int
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

	git, err := gitlab.NewClient(cfg.AuthToken, gitlab.WithBaseURL(cfg.GitlabBaseUrl))
	if err != nil {
		die(err)
	}

	pTrue := true
	pFalse := false
	opts := &gitlab.ListProjectsOptions{ListOptions: gitlab.ListOptions{PerPage: 200}, Membership: &pTrue, Archived: &pFalse}
	projects, _, err := git.Projects.ListProjects(opts)
	if err != nil {
		die(err)
	}

	for _, p := range projects {
		log.Println(p.NameWithNamespace)

		repositories, _, err := git.ContainerRegistry.ListProjectRegistryRepositories(p.ID, nil)
		if err != nil {
			log.Println(fmt.Sprintf("Skip \"%s\" repository. Error: %v", p.Name, err))
			continue
		}

		for _, r := range repositories {
			tt, _, err := git.ContainerRegistry.ListRegistryRepositoryTags(p.ID, r.ID, &gitlab.ListRegistryRepositoryTagsOptions{PerPage: 200})
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

			var reviewAppTags TagsList
			var excludedTags TagsList
			var uncategorizedTags TagsList

			for _, t := range tags {
				isIgnoredTag := false
				for _, r := range cfg.ReviewAppsTags {
					found, _ := regexp.MatchString(r, t.Name)
					if !found {
						continue
					}
					reviewAppTags = append(reviewAppTags, t)
					isIgnoredTag = true
				}
				for _, r := range cfg.ExcludedTags {
					found, _ := regexp.MatchString(r, t.Name)
					if !found {
						continue
					}
					excludedTags = append(excludedTags, t)
					isIgnoredTag = true
				}
				if !isIgnoredTag {
					uncategorizedTags = append(uncategorizedTags, t)
				}
			}
			sort.Sort(reviewAppTags)
			sort.Sort(uncategorizedTags)

			var ignoredTags TagsList // will be ignored from deletion

			// check uncategorized tags limits
			if len(uncategorizedTags) > cfg.SaveRevisionCount {
				ignoredTags = uncategorizedTags[len(uncategorizedTags)-cfg.SaveRevisionCount:]
			} else {
				ignoredTags = uncategorizedTags
			}

			// check review apps tags limits
			if len(reviewAppTags) > cfg.SaveReviewAppsCount {
				ignoredTags = append(ignoredTags, reviewAppTags[len(reviewAppTags)-cfg.SaveReviewAppsCount:]...)
			} else {
				ignoredTags = append(ignoredTags, reviewAppTags...)
			}

			ignoredTags = append(ignoredTags, excludedTags...)

			ignoredShortRevisions := make(map[string]struct{})
			for _, r := range ignoredTags {
				ignoredShortRevisions[r.ShortRevision] = struct{}{}
			}

			for _, t := range tags {
				if _, ok := ignoredShortRevisions[t.ShortRevision]; ok {
					continue
				}
				log.Println(fmt.Sprintf("Delete %s", t.Location))
				_, err := git.ContainerRegistry.DeleteRegistryRepositoryTag(p.ID, r.ID, t.Name)
				if err != nil {
					die(err)
				}
			}
		}
	}
}

// die calls log.Fatal if err wasn't nil.
func die(err error) {
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Fatal(err)
	}
}
