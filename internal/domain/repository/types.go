package repository

import (
	"analysis-module/internal/domain/analysis"
	"analysis-module/internal/domain/service"
)

type ID string
type Role string
type Language string

const (
	RoleService   Role = "service"
	RoleSharedLib Role = "shared_lib"
	RoleInfra     Role = "infra"
	RoleDocs      Role = "docs"
	RoleUnknown   Role = "unknown"

	LanguageGo     Language = "go"
	LanguagePython Language = "python"
	LanguageJS     Language = "javascript"
	LanguageTS     Language = "typescript"
	LanguageJava   Language = "java"
	LanguageYAML   Language = "yaml"
	LanguageJSON   Language = "json"
)

type BoundaryHint struct {
	Type    string `json:"type"`
	Path    string `json:"path"`
	Details string `json:"details"`
}

type TechStackProfile struct {
	Languages      []Language `json:"languages"`
	BuildFiles     []string   `json:"build_files"`
	TestFrameworks []string   `json:"test_frameworks"`
	FrameworkHints []string   `json:"framework_hints"`
}

type Manifest struct {
	ID                ID                   `json:"id"`
	Name              string               `json:"name"`
	RootPath          string               `json:"root_path"`
	Role              Role                 `json:"role"`
	IgnoreSignature   string               `json:"ignore_signature,omitempty"`
	TechStack         TechStackProfile     `json:"tech_stack"`
	GoFiles           []string             `json:"go_files"`
	PythonFiles       []string             `json:"python_files,omitempty"`
	JavaScriptFiles   []string             `json:"javascript_files,omitempty"`
	TypeScriptFiles   []string             `json:"typescript_files,omitempty"`
	ConfigFiles       []string             `json:"config_files"`
	IssueCounts       analysis.IssueCounts `json:"issue_counts,omitempty"`
	BoundaryHints     []BoundaryHint       `json:"boundary_hints"`
	CandidateServices []service.Manifest   `json:"candidate_services"`
}

type ExtractionPlan struct {
	RepositoryID ID       `json:"repository_id"`
	Language     Language `json:"language"`
	Files        []string `json:"files"`
}

type Inventory struct {
	WorkspaceID     string               `json:"workspace_id"`
	IgnoreSignature string               `json:"ignore_signature"`
	IssueCounts     analysis.IssueCounts `json:"issue_counts,omitempty"`
	Repositories    []Manifest           `json:"repositories"`
	Plans           []ExtractionPlan     `json:"plans"`
}
