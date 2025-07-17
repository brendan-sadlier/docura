package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/brendan-sadlier/docura/internal/analyser"
	"github.com/brendan-sadlier/docura/internal/generator"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	var (
		projectDir  = flag.String("dir", ".", "Project directory to analyze")
		outputDir   = flag.String("output", "./docs", "Output directory for documentation")
		configFile  = flag.String("config", "", "Configuration file")
		watch       = flag.Bool("watch", false, "Watch for changes and regenerate")
		packageName = flag.String("package", "", "Specific package to document")
	)
	flag.Parse()

	// Load configuration
	config := generator.DocConfig{
		OutputDir:        *outputDir,
		IncludePrivate:   false,
		GenerateExamples: true,
		Style:            "markdown",
	}

	if *configFile != "" {
		if err := loadConfig(*configFile, &config); err != nil {
			log.Printf("Warning: Could not load config file: %v", err)
		}
	}

	// Initialize components
	analyzer := analyser.NewAnalyser()
	generator, err := generator.NewDocGenerator()
	if err != nil {
		log.Fatal(err)
	}

	if *watch {
		if err := watchAndGenerate(analyzer, generator, *projectDir, config); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := generateDocs(analyzer, generator, *projectDir, config, *packageName); err != nil {
			log.Fatal(err)
		}
	}
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
