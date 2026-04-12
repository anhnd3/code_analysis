package jsextractor

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"analysis-module/internal/domain/symbol"
	extractorport "analysis-module/internal/ports/extractor"
	"analysis-module/pkg/ids"
)

var (
	jsImportPattern   = regexp.MustCompile(`^\s*import\s+.*from\s+['"]([^'"]+)['"]`)
	jsRequirePattern  = regexp.MustCompile(`require\(['"]([^'"]+)['"]\)`)
	jsFunctionPattern = regexp.MustCompile(`^\s*(?:export\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	jsArrowPattern    = regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_][A-Za-z0-9_]*)\s*=>`)
	jsClassPattern    = regexp.MustCompile(`^\s*(?:export\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	jsMethodPattern   = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	jsCallPattern     = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`)
)

type Extractor struct{}

func New() extractorport.SymbolExtractor {
	return &Extractor{}
}

func (e *Extractor) Supports(lang string) bool {
	return strings.EqualFold(lang, "javascript") || strings.EqualFold(lang, "typescript")
}

func (e *Extractor) ExtractFile(file symbol.FileRef) (symbol.FileExtractionResult, error) {
	handle, err := os.Open(file.AbsolutePath)
	if err != nil {
		return symbol.FileExtractionResult{}, err
	}
	defer handle.Close()

	moduleName := jsModulePath(file.RelativePath)
	result := symbol.FileExtractionResult{
		FilePath:    file.RelativePath,
		Language:    file.Language,
		PackageName: moduleName,
		Warnings:    []string{},
	}
	className := ""
	classIndent := -1
	scanner := bufio.NewScanner(handle)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		indent := leadingIndent(line)
		if className != "" && indent <= classIndent && strings.TrimSpace(line) != "}" {
			className = ""
			classIndent = -1
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if matches := jsImportPattern.FindStringSubmatch(line); len(matches) == 2 {
			result.Imports = append(result.Imports, matches[1])
		}
		for _, match := range jsRequirePattern.FindAllStringSubmatch(line, -1) {
			if len(match) == 2 {
				result.Imports = append(result.Imports, match[1])
			}
		}
		if matches := jsClassPattern.FindStringSubmatch(line); len(matches) == 2 {
			className = matches[1]
			classIndent = indent
			result.Symbols = append(result.Symbols, buildJSSymbol(file, moduleName, className, "", symbol.KindClass, line, lineNumber))
			continue
		}
		if matches := jsFunctionPattern.FindStringSubmatch(line); len(matches) == 2 {
			sym := buildJSSymbol(file, moduleName, matches[1], "", detectJSTestKind(file.RelativePath, matches[1]), line, lineNumber)
			result.Symbols = append(result.Symbols, sym)
			appendJSCalls(&result, sym, line, moduleName)
			continue
		}
		if matches := jsArrowPattern.FindStringSubmatch(line); len(matches) == 2 {
			sym := buildJSSymbol(file, moduleName, matches[1], "", detectJSTestKind(file.RelativePath, matches[1]), line, lineNumber)
			result.Symbols = append(result.Symbols, sym)
			appendJSCalls(&result, sym, line, moduleName)
			continue
		}
		if className != "" {
			if matches := jsMethodPattern.FindStringSubmatch(line); len(matches) == 2 {
				name := matches[1]
				if name != "if" && name != "for" && name != "while" && name != "switch" {
					sym := buildJSSymbol(file, moduleName, name, className, detectJSTestKind(file.RelativePath, name), line, lineNumber)
					result.Symbols = append(result.Symbols, sym)
					appendJSCalls(&result, sym, line, moduleName)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return symbol.FileExtractionResult{}, err
	}
	return result, nil
}

func buildJSSymbol(file symbol.FileRef, moduleName, name, receiver string, kind symbol.Kind, signature string, line int) symbol.Symbol {
	return symbol.Symbol{
		ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, receiver, name)),
		RepositoryID:  file.RepositoryID,
		FilePath:      file.RelativePath,
		PackageName:   moduleName,
		Name:          name,
		Receiver:      receiver,
		CanonicalName: jsCanonicalName(moduleName, receiver, name),
		Kind:          kind,
		Signature:     strings.TrimSpace(signature),
		Location: symbol.CodeLocation{
			FilePath:  file.RelativePath,
			StartLine: uint32(line),
			EndLine:   uint32(line),
		},
	}
}

func appendJSCalls(result *symbol.FileExtractionResult, sym symbol.Symbol, line, moduleName string) {
	for _, match := range jsCallPattern.FindAllStringSubmatch(line, -1) {
		target := strings.TrimSpace(match[1])
		if target == "" || target == "function" || target == "if" || target == "for" || target == "while" || target == "switch" || target == "test" || target == "it" {
			continue
		}
		result.Relations = append(result.Relations, symbol.RelationCandidate{
			SourceSymbolID:      sym.ID,
			TargetCanonicalName: resolveJSTarget(moduleName, target),
			Relationship:        "calls",
			EvidenceType:        "inline_call",
			EvidenceSource:      target,
			ExtractionMethod:    "js-regex",
			ConfidenceScore:     0.55,
		})
	}
}

func resolveJSTarget(moduleName, target string) string {
	if strings.Contains(target, ".") {
		return target
	}
	return moduleName + "." + target
}

func jsCanonicalName(moduleName, receiver, name string) string {
	if receiver != "" {
		return moduleName + "." + receiver + "." + name
	}
	return moduleName + "." + name
}

func jsModulePath(relativePath string) string {
	withoutExt := strings.TrimSuffix(relativePath, filepath.Ext(relativePath))
	return strings.ReplaceAll(withoutExt, string(filepath.Separator), ".")
}

func detectJSTestKind(path, name string) symbol.Kind {
	lower := strings.ToLower(filepath.Base(path))
	if strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") || name == "test" || name == "it" {
		return symbol.KindTestFunction
	}
	return symbol.KindFunction
}

func leadingIndent(line string) int {
	count := 0
	for _, char := range line {
		if char == ' ' {
			count++
			continue
		}
		if char == '\t' {
			count += 2
			continue
		}
		break
	}
	return count
}
