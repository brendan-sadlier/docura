package analyser

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"strings"
)

type Analyser struct {
	fset *token.FileSet
}

type PackageInfo struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Description string         `json:"description"`
	Functions   []FunctionInfo `json:"functions"`
	Types       []TypeInfo     `json:"types"`
	Constants   []ConstantInfo `json:"constants"`
	Variables   []VariableInfo `json:"variables"`
	Examples    []ExampleInfo  `json:"examples"`
	Imports     []string       `json:"imports"`
}

type FunctionInfo struct {
	Name        string       `json:"name"`
	Signature   string       `json:"signature"`
	Description string       `json:"description"`
	Parameters  []ParamInfo  `json:"parameters"`
	Returns     []ReturnInfo `json:"returns"`
	Examples    []string     `json:"examples"`
	IsExported  bool         `json:"is_exported"`
	IsMethod    bool         `json:"is_method"`
	Receiver    string       `json:"receiver,omitempty"`
}

type TypeInfo struct {
	Name        string      `json:"name"`
	Kind        string      `json:"kind"` // e.g. struct, interface, alias, etc
	Description string      `json:"description"`
	Fields      []FieldInfo `json:"fields,omitempty"`
	Methods     []string    `json:"methods,omitempty"`
	IsExported  bool        `json:"is_exported"`
}

type FieldInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Tag         string `json:"tag,omitempty"`
	Description string `json:"description"`
}

type ParamInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ReturnInfo struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ConstantInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Value       string `json:"value"`
	Description string `json:"description"`
	IsExported  bool   `json:"is_exported"`
}

type VariableInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	IsExported  bool   `json:"is_exported"`
}

type ExampleInfo struct {
	Name string `json:"name"`
	Code string `json:"code"`
	Doc  string `json:"doc"`
}

func NewAnalyser() *Analyser {
	return &Analyser{
		fset: token.NewFileSet(),
	}
}

func (a *Analyser) AnalysePackage(dir string) (*PackageInfo, error) {
	pkgs, err := parser.ParseDir(a.fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing package: %w", err)
	}

	var pkg *ast.Package
	for name, p := range pkgs {
		if !strings.HasSuffix(name, "_test") {
			pkg = p
			break
		}
	}

	if pkg == nil {
		return nil, fmt.Errorf("no Golang package found in %s", dir)
	}

	// Create Documentation
	docPkg := doc.New(pkg, "./", 0)
	info := &PackageInfo{
		Name:        docPkg.Name,
		Path:        dir,
		Description: cleanDoc(docPkg.Doc),
		Imports:     a.extractImports(pkg),
	}

	// Analyse functions
	for _, fn := range docPkg.Funcs {
		fnInfo := a.analyseFunctionDecl(fn)
		info.Functions = append(info.Functions, fnInfo)
	}

	// Analyse types
	for _, typ := range docPkg.Types {
		typeInfo := a.analyseTypeDecl(typ)
		info.Types = append(info.Types, typeInfo)

		// Add methods to functions list
		for _, method := range typ.Methods {
			methodInfo := a.analyseFunctionDecl(method)
			methodInfo.IsMethod = true
			methodInfo.Receiver = typ.Name
			info.Functions = append(info.Functions, methodInfo)
		}
	}

	// Analyse constants and variables
	for _, c := range docPkg.Consts {
		constInfo := a.analyseConstantDecl(c)
		info.Constants = append(info.Constants, constInfo...)
	}

	for _, v := range docPkg.Vars {
		varInfo := a.analyseVariableDecl(v)
		info.Variables = append(info.Variables, varInfo...)
	}

	return info, nil
}

func (a *Analyser) analyseFunctionDecl(fn *doc.Func) FunctionInfo {
	info := FunctionInfo{
		Name:        fn.Name,
		Description: cleanDoc(fn.Doc),
		IsExported:  ast.IsExported(fn.Name),
		Examples:    a.extractExamples(fn.Doc),
	}

	if fn.Decl != nil && fn.Decl.Type != nil {
		info.Signature = a.getFunctionSignature(fn.Decl)
		info.Parameters = a.extractParameters(fn.Decl.Type.Params)
		info.Returns = a.extractReturns(fn.Decl.Type.Results)
	}

	return info
}

func (a *Analyser) analyseTypeDecl(typ *doc.Type) TypeInfo {
	info := TypeInfo{
		Name:        typ.Name,
		Description: cleanDoc(typ.Doc),
		IsExported:  ast.IsExported(typ.Name),
	}

	if typ.Decl != nil {
		for _, spec := range typ.Decl.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				info.Kind = a.getTypeKind(ts.Type)
				if structType, ok := ts.Type.(*ast.StructType); ok {
					info.Fields = a.extractFields(structType)
				}
			}
		}
	}

	// Extract method names
	for _, method := range typ.Methods {
		info.Methods = append(info.Methods, method.Name)
	}

	return info
}

func (a *Analyser) analyseConstantDecl(c *doc.Value) []ConstantInfo {
	var constants []ConstantInfo

	for _, spec := range c.Decl.Specs {
		if vs, ok := spec.(*ast.ValueSpec); ok {
			for i, name := range vs.Names {
				constInfo := ConstantInfo{
					Name:        name.Name,
					Description: cleanDoc(c.Doc),
					IsExported:  ast.IsExported(name.Name),
				}

				if vs.Type != nil {
					constInfo.Type = a.typeToString(vs.Type)
				}

				if i < len(vs.Values) && vs.Values[i] != nil {
					constInfo.Value = a.exprToString(vs.Values[i])
				}

				constants = append(constants, constInfo)
			}
		}
	}

	return constants
}

func (a *Analyser) analyseVariableDecl(v *doc.Value) []VariableInfo {
	var variables []VariableInfo

	for _, spec := range v.Decl.Specs {
		if vs, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range vs.Names {
				varInfo := VariableInfo{
					Name:        name.Name,
					Description: cleanDoc(v.Doc),
					IsExported:  ast.IsExported(name.Name),
				}

				if vs.Type != nil {
					varInfo.Type = a.typeToString(vs.Type)
				}

				variables = append(variables, varInfo)
			}
		}
	}

	return variables
}

func (a *Analyser) extractImports(pkg *ast.Package) []string {
	importSet := make(map[string]bool)

	for _, file := range pkg.Files {
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			importSet[path] = true
		}
	}

	var imports []string
	for imp := range importSet {
		imports = append(imports, imp)
	}

	return imports
}

func (a *Analyser) extractParameters(fields *ast.FieldList) []ParamInfo {
	if fields == nil {
		return nil
	}

	var params []ParamInfo
	for _, field := range fields.List {
		paramType := a.typeToString(field.Type)

		if len(field.Names) == 0 {
			// Anonymous parameter
			params = append(params, ParamInfo{
				Name: "",
				Type: paramType,
			})
		} else {
			for _, name := range field.Names {
				params = append(params, ParamInfo{
					Name: name.Name,
					Type: paramType,
				})
			}
		}
	}

	return params
}

func (a *Analyser) extractReturns(fields *ast.FieldList) []ReturnInfo {
	if fields == nil {
		return nil
	}

	var returns []ReturnInfo
	for _, field := range fields.List {
		returns = append(returns, ReturnInfo{
			Type: a.typeToString(field.Type),
		})
	}

	return returns
}

func (a *Analyser) extractFields(structType *ast.StructType) []FieldInfo {
	var fields []FieldInfo

	for _, field := range structType.Fields.List {
		fieldType := a.typeToString(field.Type)
		var tag string
		if field.Tag != nil {
			tag = field.Tag.Value
		}

		if len(field.Names) == 0 {
			// Embedded field
			fields = append(fields, FieldInfo{
				Name: "",
				Type: fieldType,
				Tag:  tag,
			})
		} else {
			for _, name := range field.Names {
				fields = append(fields, FieldInfo{
					Name: name.Name,
					Type: fieldType,
					Tag:  tag,
				})
			}
		}
	}

	return fields
}

func (a *Analyser) getFunctionSignature(decl *ast.FuncDecl) string {
	// This is a simplified version - you'd want more sophisticated formatting
	var parts []string

	parts = append(parts, "func")

	if decl.Recv != nil {
		recv := a.fieldListToString(decl.Recv)
		parts = append(parts, fmt.Sprintf("(%s)", recv))
	}

	parts = append(parts, decl.Name.Name)

	if decl.Type.Params != nil {
		params := a.fieldListToString(decl.Type.Params)
		parts = append(parts, fmt.Sprintf("(%s)", params))
	} else {
		parts = append(parts, "()")
	}

	if decl.Type.Results != nil {
		results := a.fieldListToString(decl.Type.Results)
		if len(decl.Type.Results.List) == 1 && len(decl.Type.Results.List[0].Names) == 0 {
			parts = append(parts, results)
		} else {
			parts = append(parts, fmt.Sprintf("(%s)", results))
		}
	}

	return strings.Join(parts, " ")
}

func (a *Analyser) fieldListToString(fields *ast.FieldList) string {
	if fields == nil {
		return ""
	}

	var parts []string
	for _, field := range fields.List {
		fieldType := a.typeToString(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, fieldType)
		} else {
			for _, name := range field.Names {
				parts = append(parts, fmt.Sprintf("%s %s", name.Name, fieldType))
			}
		}
	}

	return strings.Join(parts, ", ")
}

func (a *Analyser) typeToString(expr ast.Expr) string {
	// Simplified type-to-string conversion
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + a.typeToString(t.X)
	case *ast.ArrayType:
		return "[]" + a.typeToString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", a.typeToString(t.Key), a.typeToString(t.Value))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", a.typeToString(t.X), t.Sel.Name)
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "unknown"
	}
}

func (a *Analyser) exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value
	case *ast.Ident:
		return e.Name
	default:
		return "..."
	}
}

func (a *Analyser) getTypeKind(expr ast.Expr) string {
	switch expr.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.ArrayType:
		return "array"
	case *ast.MapType:
		return "map"
	case *ast.ChanType:
		return "channel"
	case *ast.FuncType:
		return "function"
	default:
		return "alias"
	}
}

func (a *Analyser) extractExamples(doc string) []string {
	// Extract code examples from documentation
	var examples []string
	lines := strings.Split(doc, "\n")

	var inExample bool
	var currentExample strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "Example:") ||
			strings.HasPrefix(trimmed, "Usage:") ||
			strings.Contains(trimmed, "```go") {
			inExample = true
			currentExample.Reset()
			continue
		}

		if inExample {
			if strings.Contains(trimmed, "```") ||
				(trimmed == "" && currentExample.Len() > 0) {
				if currentExample.Len() > 0 {
					examples = append(examples, currentExample.String())
					currentExample.Reset()
				}
				inExample = false
				continue
			}

			if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
				currentExample.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "    "), "\t"))
				currentExample.WriteString("\n")
			}
		}
	}

	return examples
}

func cleanDoc(doc string) string {
	if doc == "" {
		return ""
	}

	// Remove leading/trailing whitespace and normalize line endings
	doc = strings.TrimSpace(doc)
	doc = strings.ReplaceAll(doc, "\r\n", "\n")

	// Remove common documentation artifacts
	lines := strings.Split(doc, "\n")
	var cleaned []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n")
}
