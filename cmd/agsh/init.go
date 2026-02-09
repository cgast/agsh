package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// templateDir is the default directory for spec templates.
const templateDir = "templates"

// handleInit implements `agsh init [--template=name] [--output=path]`.
func handleInit() error {
	templateName := ""
	outputPath := "project.agsh.yaml"

	for _, arg := range os.Args[2:] {
		if strings.HasPrefix(arg, "--template=") {
			templateName = strings.TrimPrefix(arg, "--template=")
		} else if strings.HasPrefix(arg, "--output=") {
			outputPath = strings.TrimPrefix(arg, "--output=")
		}
	}

	if templateName == "" {
		return listTemplates()
	}

	return scaffoldFromTemplate(templateName, outputPath)
}

// listTemplates shows available templates.
func listTemplates() error {
	fmt.Println("Usage: agsh init --template=<name> [--output=<path>]")
	fmt.Println()
	fmt.Println("Available templates:")

	templates, err := findTemplates()
	if err != nil {
		fmt.Println("  (no templates found)")
		return nil
	}

	for _, t := range templates {
		fmt.Printf("  - %s\n", t)
	}

	return nil
}

// findTemplates discovers available template names.
func findTemplates() ([]string, error) {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			name := strings.TrimSuffix(e.Name(), ".yaml")
			names = append(names, name)
		}
	}
	return names, nil
}

// scaffoldFromTemplate copies a template to the output path.
func scaffoldFromTemplate(name, outputPath string) error {
	templatePath := filepath.Join(templateDir, name+".yaml")

	data, err := os.ReadFile(templatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("template %q not found (looked in %s)", name, templatePath)
		}
		return fmt.Errorf("read template: %w", err)
	}

	// Check if output already exists.
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("file %q already exists (use --output to specify a different path)", outputPath)
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(outputPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	fmt.Printf("Created %s from template %q\n", outputPath, name)
	fmt.Println("Edit the file to customize your project spec, then run:")
	fmt.Printf("  agsh run %s\n", outputPath)

	return nil
}
