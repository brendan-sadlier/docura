package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/brendan-sadlier/docura/internal/analyser"
	"github.com/brendan-sadlier/docura/internal/generator"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	projectDir    string
	docsOutputDir string
	configFile    string
	watch         bool
	packageName   string
)
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "generate documentation",
	Long:  `generate Markdown documentation for Golang packages`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runGenerate(); err != nil {
			log.Fatalf("generate failed: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().StringVarP(&projectDir, "directory", "d", "", "Project directory to generate documentation")
	generateCmd.Flags().StringVarP(&docsOutputDir, "output", "o", "./docs", "Output directory for generated documentation [default ./docs]")
	generateCmd.Flags().StringVarP(&configFile, "config", "c", "", "Configuration file in JSON format")
	generateCmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch for changes to the documentation")
	generateCmd.Flags().StringVarP(&packageName, "package", "p", "", "Specific package to analyse")
}

func runGenerate() error {
	// Default config values
	config := generator.DocConfig{
		OutputDir:        docsOutputDir,
		IncludePrivate:   false,
		GenerateExamples: true,
		Style:            "markdown",
	}

	// Load config file if specified
	if configFile != "" {
		if err := loadConfig(configFile, &config); err != nil {
			log.Printf("Could not load config file %s, proceding with default: %v ", configFile, err)
		}
	}

	analyserInstance := analyser.NewAnalyser()
	docGenerator, err := generator.NewDocGenerator()
	if err != nil {
		log.Fatalf("Could not create document generator: %v", err)
	}

	if watch {
		return watchAndGenerate(analyserInstance, docGenerator, projectDir, config)
	}

	return generateDocs(analyserInstance, docGenerator, projectDir, config, packageName)
}

func generateDocs(analyser *analyser.Analyser, generator *generator.DocGenerator, projectDir string, config generator.DocConfig, packageName string) error {
	if packageName != "" {
		// Document specific package
		return generatePackageDocs(analyser, generator, filepath.Join(projectDir, packageName), config)
	}

	// Document all packages
	return filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			return nil
		}

		// Skip vendor, .git, and test directories
		if shouldSkipDir(path) {
			return filepath.SkipDir
		}

		// Check if directory contains Go files
		hasGoFiles, err := hasGoSourceFiles(path)
		if err != nil {
			return err
		}

		if hasGoFiles {
			if err := generatePackageDocs(analyser, generator, path, config); err != nil {
				log.Printf("Error documenting package %s: %v", path, err)
			}
		}

		return nil
	})
}

func watchAndGenerate(analyser *analyser.Analyser, generator *generator.DocGenerator, projectDir string, config generator.DocConfig) error {
	// Simplified file watching - you'd want to use fsnotify for production
	fmt.Printf("Watching %s for changes...\n", projectDir)

	for {
		if err := generateDocs(analyser, generator, projectDir, config, ""); err != nil {
			log.Printf("Error generating docs: %v", err)
		}
		time.Sleep(30 * time.Second)
	}
}

func generatePackageDocs(analyser *analyser.Analyser, generator *generator.DocGenerator, packageDir string, config generator.DocConfig) error {
	fmt.Printf("Analyzing package: %s\n", packageDir)

	// Analyze package
	pkg, err := analyser.AnalysePackage(packageDir)
	if err != nil {
		return fmt.Errorf("analyzing package: %w", err)
	}

	// Generate documentation
	doc, err := generator.GeneratePackageDoc(pkg, config)
	if err != nil {
		return fmt.Errorf("generating documentation: %w", err)
	}

	// Write to file
	outputPath := filepath.Join(config.OutputDir, pkg.Name+".md")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(doc), 0644); err != nil {
		return fmt.Errorf("writing documentation: %w", err)
	}

	fmt.Printf("Generated documentation: %s\n", outputPath)
	return nil
}

func shouldSkipDir(path string) bool {
	base := filepath.Base(path)
	return base == "vendor" ||
		base == ".git" ||
		base == "testdata" ||
		strings.HasSuffix(base, "_test")
}

func hasGoSourceFiles(dir string) (bool, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".go") &&
			!strings.HasSuffix(file.Name(), "_test.go") {
			return true, nil
		}
	}

	return false, nil
}

func loadConfig(filename string, config *generator.DocConfig) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, config)
}
