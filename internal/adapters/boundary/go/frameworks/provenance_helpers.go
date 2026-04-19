package frameworks

import (
	boundary "analysis-module/internal/adapters/boundary/go"
	"path/filepath"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func fileImportAliases(file boundary.ParsedGoFile) map[string]string {
	aliases := map[string]string{}
	walk(file.Root, func(node *tree_sitter.Node) bool {
		if node.Kind() != "import_spec" {
			return true
		}
		pathNode := node.ChildByFieldName("path")
		if pathNode == nil {
			return false
		}
		importPath := strings.Trim(nodeText(pathNode, file.Content), "\"")
		alias := filepath.Base(importPath)
		if aliasNode := node.ChildByFieldName("name"); aliasNode != nil {
			alias = nodeText(aliasNode, file.Content)
		}
		aliases[alias] = importPath
		return false
	})
	return aliases
}

func importAliasMatches(aliases map[string]string, alias, expectedPath string) bool {
	return aliases[alias] == expectedPath
}

func importAliasMatchesFunc(aliases map[string]string, alias string, match func(string) bool) bool {
	path, ok := aliases[alias]
	return ok && match(path)
}

func declarationParameterTypes(node *tree_sitter.Node, content []byte) map[string]string {
	types := map[string]string{}
	params := node.ChildByFieldName("parameters")
	if params == nil {
		return types
	}

	for i := 0; i < int(params.NamedChildCount()); i++ {
		param := params.NamedChild(uint(i))
		if param == nil || param.Kind() != "parameter_declaration" {
			continue
		}
		typeNode := param.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}
		typeText := strings.TrimSpace(nodeText(typeNode, content))
		nameNode := param.ChildByFieldName("name")
		if nameNode != nil {
			for _, item := range expressionItems(nameNode) {
				if name := identifierName(item, content); name != "" {
					types[name] = typeText
				}
			}
			continue
		}
		if param.NamedChildCount() > 1 {
			for j := 0; j < int(param.NamedChildCount())-1; j++ {
				if name := identifierName(param.NamedChild(uint(j)), content); name != "" {
					types[name] = typeText
				}
			}
		}
	}
	return types
}

func matchesQualifiedType(rawType string, aliases map[string]string, expectedAliasPath, typeName string) bool {
	rawType = strings.TrimSpace(rawType)
	rawType = strings.TrimPrefix(rawType, "*")
	qualified := "." + typeName
	if !strings.HasSuffix(rawType, qualified) {
		return false
	}
	alias := strings.TrimSuffix(rawType, qualified)
	return importAliasMatches(aliases, alias, expectedAliasPath)
}

func matchesQualifiedTypeFunc(rawType string, aliases map[string]string, typeName string, match func(string) bool) bool {
	rawType = strings.TrimSpace(rawType)
	rawType = strings.TrimPrefix(rawType, "*")
	qualified := "." + typeName
	if !strings.HasSuffix(rawType, qualified) {
		return false
	}
	alias := strings.TrimSuffix(rawType, qualified)
	return importAliasMatchesFunc(aliases, alias, match)
}

func structFieldsMatchingType(files []boundary.ParsedGoFile, matchesType func(string, map[string]string) bool) map[string]map[string]bool {
	fieldsByType := map[string]map[string]bool{}
	for _, file := range files {
		aliases := fileImportAliases(file)
		walk(file.Root, func(node *tree_sitter.Node) bool {
			if node.Kind() != "type_spec" {
				return true
			}
			nameNode := node.ChildByFieldName("name")
			typeNode := node.ChildByFieldName("type")
			if nameNode == nil || typeNode == nil || typeNode.Kind() != "struct_type" {
				return false
			}

			typeName := nodeText(nameNode, file.Content)
			walk(typeNode, func(child *tree_sitter.Node) bool {
				if child.Kind() != "field_declaration" {
					return true
				}
				fieldType := fieldDeclarationType(child, file.Content)
				if fieldType == "" || !matchesType(fieldType, aliases) {
					return false
				}
				for _, fieldName := range fieldDeclarationNames(child, file.Content) {
					if fieldsByType[typeName] == nil {
						fieldsByType[typeName] = map[string]bool{}
					}
					fieldsByType[typeName][fieldName] = true
				}
				return false
			})
			return false
		})
	}
	return fieldsByType
}

func fieldDeclarationType(node *tree_sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		return strings.TrimSpace(nodeText(typeNode, content))
	}
	if node.NamedChildCount() == 0 {
		return ""
	}
	return strings.TrimSpace(nodeText(node.NamedChild(uint(node.NamedChildCount()-1)), content))
}

func fieldDeclarationNames(node *tree_sitter.Node, content []byte) []string {
	if node == nil {
		return nil
	}
	typeNode := node.ChildByFieldName("type")
	names := []string{}
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		for _, item := range expressionItems(nameNode) {
			if name := fieldNameFromNode(item, content); name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		if child == nil || child == typeNode {
			continue
		}
		switch child.Kind() {
		case "identifier", "field_identifier":
			names = append(names, nodeText(child, content))
		case "identifier_list":
			for _, item := range expressionItems(child) {
				if name := fieldNameFromNode(item, content); name != "" {
					names = append(names, name)
				}
			}
		}
	}
	return names
}

func copyBoolScope(scope map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(scope))
	for key, value := range scope {
		cloned[key] = value
	}
	return cloned
}
