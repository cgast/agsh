package verify

import (
	"fmt"

	agshctx "github.com/cgast/agsh/pkg/context"
)

// LLMJudgeEndpoint is the URL for the LLM judge service.
// When empty, llm_judge assertions are skipped with a pass.
var LLMJudgeEndpoint string

func init() {
	RegisterChecker("llm_judge", checkLLMJudge)
}

// checkLLMJudge evaluates output against the intent description using an LLM.
// When no endpoint is configured, it returns a pass with a skip message.
func checkLLMJudge(envelope agshctx.Envelope, assertion Assertion) AssertionResult {
	if LLMJudgeEndpoint == "" {
		return AssertionResult{
			Assertion: assertion,
			Passed:    true,
			Message:   "llm_judge: skipped (no endpoint configured)",
		}
	}

	// When an endpoint is configured, this would:
	// 1. Send the intent description + output to the LLM
	// 2. Ask "does this output satisfy the intent?"
	// 3. Parse the LLM's yes/no response
	//
	// For now, return a placeholder that indicates the endpoint exists
	// but the full implementation is not yet wired.
	return AssertionResult{
		Assertion: assertion,
		Passed:    false,
		Message:   fmt.Sprintf("llm_judge: endpoint %q configured but not yet implemented", LLMJudgeEndpoint),
	}
}
