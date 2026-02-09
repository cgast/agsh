package spec

// ProjectSpec defines a complete task specification that an agent executes against.
// It is the contract between human intent and agent execution.
type ProjectSpec struct {
	APIVersion      string      `yaml:"apiVersion" json:"apiVersion"`
	Kind            string      `yaml:"kind" json:"kind"`
	Meta            SpecMeta    `yaml:"meta" json:"meta"`
	Goal            string      `yaml:"goal" json:"goal"`
	Constraints     []string    `yaml:"constraints" json:"constraints"`
	Guidelines      []string    `yaml:"guidelines" json:"guidelines"`
	SuccessCriteria []Assertion `yaml:"success_criteria" json:"success_criteria"`
	AllowedCommands []string    `yaml:"allowed_commands" json:"allowed_commands"`
	Output          OutputSpec  `yaml:"output" json:"output"`
	Params          []ParamDef  `yaml:"params" json:"params"`
}

// SpecMeta contains metadata about the spec.
type SpecMeta struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Author      string   `yaml:"author" json:"author"`
	Created     string   `yaml:"created" json:"created"`
	Tags        []string `yaml:"tags" json:"tags"`
}

// OutputSpec describes the expected output.
type OutputSpec struct {
	Path   string `yaml:"path" json:"path"`
	Format string `yaml:"format" json:"format"`
}

// ParamDef defines a runtime parameter that the human provides.
type ParamDef struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type" json:"type"`
	Default     any    `yaml:"default" json:"default"`
	Description string `yaml:"description" json:"description"`
}

// Assertion defines a machine-checkable condition for verification.
// This type is compatible with pkg/verify.Assertion (Phase 3).
type Assertion struct {
	Type     string `yaml:"type" json:"type"`         // "contains", "not_empty", "json_schema", "count_gte", "matches_regex", "llm_judge"
	Target   string `yaml:"target" json:"target"`     // what to check: "output", "context.session.x", etc.
	Expected any    `yaml:"expected" json:"expected"` // the expected value/pattern
	Message  string `yaml:"message" json:"message"`   // human-readable failure description
}
