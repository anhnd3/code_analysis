package repo_inventory

import (
	"sort"
	"strings"

	"analysis-module/internal/domain/repository"
	scannerport "analysis-module/internal/ports/scanner"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) Build(workspaceID string, scan scannerport.ScanWorkspaceResult) repository.Inventory {
	repos := make([]repository.Manifest, 0, len(scan.Repositories))
	plans := []repository.ExtractionPlan{}
	for _, repo := range scan.Repositories {
		repo.Role = classifyRole(repo)
		repos = append(repos, repo)
		for _, lang := range repo.TechStack.Languages {
			if lang == repository.LanguageGo && len(repo.GoFiles) > 0 {
				plans = append(plans, repository.ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.GoFiles...),
				})
			}
			if lang == repository.LanguagePython && len(repo.PythonFiles) > 0 {
				plans = append(plans, repository.ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.PythonFiles...),
				})
			}
			if lang == repository.LanguageJS && len(repo.JavaScriptFiles) > 0 {
				plans = append(plans, repository.ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.JavaScriptFiles...),
				})
			}
			if lang == repository.LanguageTS && len(repo.TypeScriptFiles) > 0 {
				plans = append(plans, repository.ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.TypeScriptFiles...),
				})
			}
		}
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].RootPath < repos[j].RootPath })
	return repository.Inventory{
		WorkspaceID:  workspaceID,
		Repositories: repos,
		Plans:        plans,
	}
}

func classifyRole(repo repository.Manifest) repository.Role {
	if len(repo.CandidateServices) > 0 {
		return repository.RoleService
	}
	if len(repo.GoFiles) > 0 && len(repo.PythonFiles) == 0 && len(repo.JavaScriptFiles) == 0 && len(repo.TypeScriptFiles) == 0 {
		return repository.RoleSharedLib
	}
	if len(repo.GoFiles) > 0 || len(repo.PythonFiles) > 0 || len(repo.JavaScriptFiles) > 0 || len(repo.TypeScriptFiles) > 0 {
		if len(repo.TechStack.BuildFiles) > 0 {
			return repository.RoleService
		}
	}
	for _, buildFile := range repo.TechStack.BuildFiles {
		lower := strings.ToLower(buildFile)
		if strings.Contains(lower, "terraform") || strings.Contains(lower, "docker") {
			return repository.RoleInfra
		}
	}
	return repository.RoleUnknown
}
