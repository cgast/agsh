package spec

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadSpec reads a YAML spec file and returns a parsed ProjectSpec.
// Template variables like {{date}} and {{param_name}} are interpolated
// using the provided params (or defaults from the spec).
func LoadSpec(path string, params map[string]string) (ProjectSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectSpec{}, fmt.Errorf("read spec %s: %w", path, err)
	}

	return ParseSpec(data, params)
}

// ParseSpec parses YAML data into a ProjectSpec with variable interpolation.
func ParseSpec(data []byte, params map[string]string) (ProjectSpec, error) {
	// First pass: parse to get param defaults.
	var raw ProjectSpec
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ProjectSpec{}, fmt.Errorf("parse spec: %w", err)
	}

	// Build interpolation map from param defaults + overrides.
	vars := buildVarMap(raw.Params, params)

	// Interpolate variables in the raw YAML.
	interpolated := interpolateVars(string(data), vars)

	// Second pass: parse the interpolated YAML.
	var spec ProjectSpec
	if err := yaml.Unmarshal([]byte(interpolated), &spec); err != nil {
		return ProjectSpec{}, fmt.Errorf("parse interpolated spec: %w", err)
	}

	return spec, nil
}

// buildVarMap creates a variable map from param defaults and runtime overrides.
// Built-in variables like {{date}} are always available.
func buildVarMap(paramDefs []ParamDef, overrides map[string]string) map[string]string {
	vars := make(map[string]string)

	// Built-in variables.
	now := time.Now()
	vars["date"] = now.Format("2006-01-02")
	vars["datetime"] = now.Format("2006-01-02T15:04:05")
	vars["year"] = now.Format("2006")
	vars["month"] = now.Format("01")
	vars["day"] = now.Format("02")

	// Param defaults.
	for _, p := range paramDefs {
		if p.Default != nil {
			vars[p.Name] = fmt.Sprintf("%v", p.Default)
		}
	}

	// Runtime overrides.
	for k, v := range overrides {
		vars[k] = v
	}

	return vars
}

// templatePattern matches {{var_name}} patterns.
var templatePattern = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)

// interpolateVars replaces {{var_name}} patterns with values from the var map.
func interpolateVars(s string, vars map[string]string) string {
	return templatePattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		if val, ok := vars[varName]; ok {
			return val
		}
		return match // Leave unresolved.
	})
}
