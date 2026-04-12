package pythonextractor

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
	pythonImportPattern     = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_., ]+)`)
	pythonFromImportPattern = regexp.MustCompile(`^\s*from\s+([A-Za-z0-9_\.]+)\s+import\s+(.+)$`)
	pythonFunctionPattern   = regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	pythonClassPattern      = regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	pythonCallPattern       = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_\.]*)\s*\(`)
)

type Extractor struct{}

func New() extractorport.SymbolExtractor {
	return &Extractor{}
}

func (e *Extractor) Supports(lang string) bool {
	return strings.EqualFold(lang, "python")
}

func (e *Extractor) ExtractFile(file symbol.FileRef) (symbol.FileExtractionResult, error) {
	handle, err := os.Open(file.AbsolutePath)
	if err != nil {
		return symbol.FileExtractionResult{}, err
	}
	defer handle.Close()

	moduleName := pythonModulePath(file.RelativePath)
	result := symbol.FileExtractionResult{
		FilePath:    file.RelativePath,
		Language:    "python",
		PackageName: moduleName,
		Warnings:    []string{},
	}
	scanner := bufio.NewScanner(handle)
	lineNumber := 0
	classStack := []classFrame{}
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		indent := leadingIndent(line)
		for len(classStack) > 0 && indent <= classStack[len(classStack)-1].indent {
			classStack = classStack[:len(classStack)-1]
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if matches := pythonImportPattern.FindStringSubmatch(line); len(matches) == 2 {
			for _, item := range strings.Split(matches[1], ",") {
				imported := strings.TrimSpace(item)
				if imported != "" {
					result.Imports = append(result.Imports, imported)
				}
			}
		}
		if matches := pythonFromImportPattern.FindStringSubmatch(line); len(matches) == 3 {
			module := strings.TrimSpace(matches[1])
			if module != "" {
				result.Imports = append(result.Imports, module)
			}
		}
		if matches := pythonClassPattern.FindStringSubmatch(line); len(matches) == 2 {
			name := matches[1]
			classStack = append(classStack, classFrame{name: name, indent: indent})
			result.Symbols = append(result.Symbols, buildPythonSymbol(file, moduleName, name, "", symbol.KindClass, line, lineNumber))
			continue
		}
		if matches := pythonFunctionPattern.FindStringSubmatch(line); len(matches) == 2 {
			name := matches[1]
			receiver := ""
			kind := symbol.KindFunction
			if len(classStack) > 0 {
				receiver = classStack[len(classStack)-1].name
				kind = symbol.KindMethod
			}
			if strings.HasPrefix(name, "test_") || strings.HasPrefix(filepath.Base(file.RelativePath), "test_") || strings.HasSuffix(filepath.Base(file.RelativePath), "_test.py") {
				kind = symbol.KindTestFunction
			}
			sym := buildPythonSymbol(file, moduleName, name, receiver, kind, line, lineNumber)
			result.Symbols = append(result.Symbols, sym)
			appendPythonCalls(&result, sym, line, moduleName)
		}
	}
	if err := scanner.Err(); err != nil {
		return symbol.FileExtractionResult{}, err
	}
	return result, nil
}

type classFrame struct {
	name   string
	indent int
}

func buildPythonSymbol(file symbol.FileRef, moduleName, name, receiver string, kind symbol.Kind, signature string, line int) symbol.Symbol {
	return symbol.Symbol{
		ID:            symbol.ID(ids.Stable("sym", file.RepositoryID, file.RelativePath, receiver, name)),
		RepositoryID:  file.RepositoryID,
		FilePath:      file.RelativePath,
		PackageName:   moduleName,
		Name:          name,
		Receiver:      receiver,
		CanonicalName: pythonCanonicalName(moduleName, receiver, name),
		Kind:          kind,
		Signature:     strings.TrimSpace(signature),
		Location: symbol.CodeLocation{
			FilePath:  file.RelativePath,
			StartLine: uint32(line),
			EndLine:   uint32(line),
		},
	}
}

func appendPythonCalls(result *symbol.FileExtractionResult, sym symbol.Symbol, line, moduleName string) {
	for _, match := range pythonCallPattern.FindAllStringSubmatch(line, -1) {
		target := strings.TrimSpace(match[1])
		if target == "" || target == "def" || target == "class" {
			continue
		}
		result.Relations = append(result.Relations, symbol.RelationCandidate{
			SourceSymbolID:      sym.ID,
			TargetCanonicalName: resolvePythonTarget(moduleName, target),
			Relationship:        "calls",
			EvidenceType:        "inline_call",
			EvidenceSource:      target,
			ExtractionMethod:    "python-regex",
			ConfidenceScore:     0.6,
		})
	}
}

func resolvePythonTarget(moduleName, target string) string {
	if strings.Contains(target, ".") {
		return target
	}
	return moduleName + "." + target
}

func pythonCanonicalName(moduleName, receiver, name string) string {
	if receiver != "" {
		return moduleName + "." + receiver + "." + name
	}
	return moduleName + "." + name
}

func pythonModulePath(relativePath string) string {
	withoutExt := strings.TrimSuffix(relativePath, filepath.Ext(relativePath))
	withoutInit := strings.TrimSuffix(withoutExt, string(filepath.Separator)+"__init__")
	withoutInit = strings.TrimSuffix(withoutInit, ".__init__")
	return strings.ReplaceAll(withoutInit, string(filepath.Separator), ".")
}

func leadingIndent(line string) int {
	count := 0
	for _, char := range line {
		if char == ' ' {
			count++
			continue
		}
		if char == '\t' {
			count += 4
			continue
		}
		break
	}
	return count
}
