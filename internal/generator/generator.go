package generator

import (
	"context"
	"fmt"
	"github.com/brendan-sadlier/docura/internal/analyser"
	"os"
	"strings"
	"text/template"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
)

type DocGenerator struct {
	llm       llms.Model
	templates map[string]*template.Template
}

type DocConfig struct {
	ProjectName      string `json:"project_name"`
	ProjectDesc      string `json:"project_description"`
	OutputDir        string `json:"output_dir"`
	IncludePrivate   bool   `json:"include_private"`
	GenerateExamples bool   `json:"generate_examples"`
	Style            string `json:"style"` // "godoc", "markdown", "html"
}

func NewDocGenerator() (*DocGenerator, error) {
	llm, err := openai.New(
		openai.WithModel("llama3-8b-8192"),
		openai.WithBaseURL("https://api.groq.com/openai/v1"),
		openai.WithToken(os.Getenv("GROQ_API_KEY")),
	)
	if err != nil {
		return nil, fmt.Errorf("creating LLM: %w", err)
	}

	dg := &DocGenerator{
		llm:       llm,
		templates: make(map[string]*template.Template),
	}

	if err := dg.loadTemplates(); err != nil {
		return nil, fmt.Errorf("loading templates: %w", err)
	}

	return dg, nil
}

func (dg *DocGenerator) loadTemplates() error {
	// Package documentation template
	packageTmpl := `# {{.Name}}

{{.Description}}

## Installation

'''bash
go get {{.Path}}
'''

## Usage

{{if .Examples}}
{{range .Examples}}
'''go
{{.Code}}
'''
{{end}}
{{end}}

## API Reference

{{if .Functions}}
### Functions

{{range .Functions}}
{{if .IsExported}}
#### {{.Name}}

'''go
{{.Signature}}
'''

{{.Description}}

{{if .Parameters}}
**Parameters:**
{{range .Parameters}}
- '{{.Name}}' ({{.Type}})
{{end}}
{{end}}

{{if .Returns}}
**Returns:**
{{range .Returns}}
- {{.Type}}{{if .Description}} - {{.Description}}{{end}}
{{end}}
{{end}}

{{if .Examples}}
**Example:**
{{range .Examples}}
'''go
{{.}}
'''
{{end}}
{{end}}

{{end}}
{{end}}
{{end}}

{{if .Types}}
### Types

{{range .Types}}
{{if .IsExported}}
#### {{.Name}}

'''go
type {{.Name}} {{.Kind}}
'''

{{.Description}}

{{if .Fields}}
**Fields:**
{{range .Fields}}
- '{{.Name}}' {{.Type}}{{if .Description}} - {{.Description}}{{end}}
{{end}}
{{end}}

{{if .Methods}}
**Methods:**
{{range .Methods}}
- [{{.}}](#{{.}})
{{end}}
{{end}}

{{end}}
{{end}}
{{end}}
`

	tmpl, err := template.New("package").Parse(packageTmpl)
	if err != nil {
		return fmt.Errorf("parsing package template: %w", err)
	}
	dg.templates["package"] = tmpl

	return nil
}

func (dg *DocGenerator) GeneratePackageDoc(pkg *analyser.PackageInfo, config DocConfig) (string, error) {
	// Enhance descriptions with AI
	if err := dg.enhanceDescriptions(pkg); err != nil {
		return "", fmt.Errorf("enhancing descriptions: %w", err)
	}

	// Generate usage examples
	if config.GenerateExamples {
		if err := dg.generateExamples(pkg); err != nil {
			return "", fmt.Errorf("generating examples: %w", err)
		}
	}

	// Apply template
	var result strings.Builder
	if err := dg.templates["package"].Execute(&result, pkg); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return result.String(), nil
}

func (dg *DocGenerator) enhanceDescriptions(pkg *analyser.PackageInfo) error {
	ctx := context.Background()

	// Enhance package description if empty or too brief
	if len(pkg.Description) < 50 {
		enhanced, err := dg.enhancePackageDescription(ctx, pkg)
		if err == nil && enhanced != "" {
			pkg.Description = enhanced
		}
	}

	// Enhance function descriptions
	for i := range pkg.Functions {
		if len(pkg.Functions[i].Description) < 20 {
			enhanced, err := dg.enhanceFunctionDescription(ctx, &pkg.Functions[i])
			if err == nil && enhanced != "" {
				pkg.Functions[i].Description = enhanced
			}
		}
	}

	// Enhance type descriptions
	for i := range pkg.Types {
		if len(pkg.Types[i].Description) < 20 {
			enhanced, err := dg.enhanceTypeDescription(ctx, &pkg.Types[i])
			if err == nil && enhanced != "" {
				pkg.Types[i].Description = enhanced
			}
		}
	}

	return nil
}

func (dg *DocGenerator) enhancePackageDescription(ctx context.Context, pkg *analyser.PackageInfo) (string, error) {
	template := prompts.NewPromptTemplate(`
Analyze this Go package and write a clear, concise description (2-3 sentences):

Package: {{.name}}
Path: {{.path}}

Functions: {{range .functions}}{{.name}}, {{end}}
Types: {{range .types}}{{.name}}, {{end}}

Write a professional description that explains:
1. What this package does
2. Who would use it
3. Key capabilities

Keep it under 200 words and avoid marketing language.`,
		[]string{"name", "path", "functions", "types"})

	prompt, err := template.Format(map[string]any{
		"name":      pkg.Name,
		"path":      pkg.Path,
		"functions": pkg.Functions,
		"types":     pkg.Types,
	})
	if err != nil {
		return "", err
	}

	response, err := dg.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.Choices[0].Content), nil
}

func (dg *DocGenerator) enhanceFunctionDescription(ctx context.Context, fn *analyser.FunctionInfo) (string, error) {
	template := prompts.NewPromptTemplate(`
Write a clear description for this Go function:

Function: {{.name}}
Signature: {{.signature}}
{{if .parameters}}Parameters: {{range .parameters}}{{.name}} {{.type}}, {{end}}{{end}}
{{if .returns}}Returns: {{range .returns}}{{.type}}, {{end}}{{end}}

Describe what it does, when to use it, and any important behavior.
Keep it concise (1-2 sentences).`,
		[]string{"name", "signature", "parameters", "returns"})

	prompt, err := template.Format(map[string]any{
		"name":       fn.Name,
		"signature":  fn.Signature,
		"parameters": fn.Parameters,
		"returns":    fn.Returns,
	})
	if err != nil {
		return "", err
	}

	response, err := dg.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.Choices[0].Content), nil
}

func (dg *DocGenerator) enhanceTypeDescription(ctx context.Context, typ *analyser.TypeInfo) (string, error) {
	template := prompts.NewPromptTemplate(`
Write a clear description for this Go type:

Type: {{.name}} ({{.kind}})
{{if .fields}}Fields: {{range .fields}}{{.name}} {{.type}}, {{end}}{{end}}
{{if .methods}}Methods: {{range .methods}}{{.}}, {{end}}{{end}}

Describe what it represents and how it's used.
Keep it concise (1-2 sentences).`,
		[]string{"name", "kind", "fields", "methods"})

	prompt, err := template.Format(map[string]any{
		"name":    typ.Name,
		"kind":    typ.Kind,
		"fields":  typ.Fields,
		"methods": typ.Methods,
	})
	if err != nil {
		return "", err
	}

	response, err := dg.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.Choices[0].Content), nil
}

func (dg *DocGenerator) generateExamples(pkg *analyser.PackageInfo) error {
	ctx := context.Background()

	// Generate package-level usage example
	if len(pkg.Examples) == 0 {
		example, err := dg.generatePackageExample(ctx, pkg)
		if err == nil && example != "" {
			pkg.Examples = append(pkg.Examples, analyser.ExampleInfo{
				Name: "Basic Usage",
				Code: example,
				Doc:  "Basic usage example",
			})
		}
	}

	// Generate function examples
	for i := range pkg.Functions {
		if len(pkg.Functions[i].Examples) == 0 && pkg.Functions[i].IsExported {
			example, err := dg.generateFunctionExample(ctx, &pkg.Functions[i], pkg)
			if err == nil && example != "" {
				pkg.Functions[i].Examples = append(pkg.Functions[i].Examples, example)
			}
		}
	}

	return nil
}

func (dg *DocGenerator) generatePackageExample(ctx context.Context, pkg *analyser.PackageInfo) (string, error) {
	template := prompts.NewPromptTemplate(`
Create a realistic Go code example showing how to use this package:

Package: {{.name}}
Description: {{.description}}
Key Functions: {{range .functions}}{{if .is_exported}}{{.name}}, {{end}}{{end}}
Key Types: {{range .types}}{{if .is_exported}}{{.name}}, {{end}}{{end}}

Write a complete, runnable example that shows:
1. Import statement
2. Basic usage
3. Error handling
4. Realistic use case

Return only the Go code, no explanations.`,
		[]string{"name", "description", "functions", "types"})

	prompt, err := template.Format(map[string]any{
		"name":        pkg.Name,
		"description": pkg.Description,
		"functions":   pkg.Functions,
		"types":       pkg.Types,
	})
	if err != nil {
		return "", err
	}

	response, err := dg.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.Choices[0].Content), nil
}

func (dg *DocGenerator) generateFunctionExample(ctx context.Context, fn *analyser.FunctionInfo, pkg *analyser.PackageInfo) (string, error) {
	template := prompts.NewPromptTemplate(`
Create a Go code example for this function:

Function: {{.name}}
Signature: {{.signature}}
Package: {{.package}}
{{if .parameters}}Parameters: {{range .parameters}}{{.name}} {{.type}}, {{end}}{{end}}

Write a realistic example showing how to call this function.
Include proper error handling if needed.
Return only the Go code snippet.`,
		[]string{"name", "signature", "package", "parameters"})

	prompt, err := template.Format(map[string]any{
		"name":       fn.Name,
		"signature":  fn.Signature,
		"package":    pkg.Name,
		"parameters": fn.Parameters,
	})
	if err != nil {
		return "", err
	}

	response, err := dg.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.Choices[0].Content), nil
}
