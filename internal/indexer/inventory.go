package indexer

import (
	"sort"
	"strings"
)

// InventoryBuilder builds repository inventories from scan results.
type InventoryBuilder struct{}

func NewInventoryBuilder() InventoryBuilder {
	return InventoryBuilder{}
}

// Build constructs a Inventory from workspace scan output.
func (InventoryBuilder) Build(workspaceID string, scan ScanWorkspaceResult) Inventory {
	repos := make([]Manifest, 0, len(scan.Repositories))
	plans := []ExtractionPlan{}
	issueCounts := IssueCounts{}
	ignoreSignature := ""
	for _, repo := range scan.Repositories {
		repo.Role = classifyRole(repo)
		issueCounts.Add(repo.IssueCounts)
		if ignoreSignature == "" {
			ignoreSignature = repo.IgnoreSignature
		}
		repos = append(repos, repo)
		for _, lang := range repo.TechStack.Languages {
			if lang == LanguageGo && len(repo.GoFiles) > 0 {
				plans = append(plans, ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.GoFiles...),
				})
			}
			if lang == LanguagePython && len(repo.PythonFiles) > 0 {
				plans = append(plans, ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.PythonFiles...),
				})
			}
			if lang == LanguageJS && len(repo.JavaScriptFiles) > 0 {
				plans = append(plans, ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.JavaScriptFiles...),
				})
			}
			if lang == LanguageTS && len(repo.TypeScriptFiles) > 0 {
				plans = append(plans, ExtractionPlan{
					RepositoryID: repo.ID,
					Language:     lang,
					Files:        append([]string(nil), repo.TypeScriptFiles...),
				})
			}
		}
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].RootPath < repos[j].RootPath })
	return Inventory{
		WorkspaceID:     workspaceID,
		IgnoreSignature: ignoreSignature,
		IssueCounts:     issueCounts,
		Repositories:    repos,
		Plans:           plans,
	}
}

func classifyRole(repo Manifest) RepoRole {
	if len(repo.CandidateServices) > 0 {
		return RoleService
	}
	if len(repo.GoFiles) > 0 && len(repo.PythonFiles) == 0 && len(repo.JavaScriptFiles) == 0 && len(repo.TypeScriptFiles) == 0 {
		return RoleSharedLib
	}
	if len(repo.GoFiles) > 0 || len(repo.PythonFiles) > 0 || len(repo.JavaScriptFiles) > 0 || len(repo.TypeScriptFiles) > 0 {
		if len(repo.TechStack.BuildFiles) > 0 {
			return RoleService
		}
	}
	for _, buildFile := range repo.TechStack.BuildFiles {
		lower := strings.ToLower(buildFile)
		if strings.Contains(lower, "terraform") || strings.Contains(lower, "docker") {
			return RoleInfra
		}
	}
	return RoleUnknown
}
