package condition

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/lyzr/orchestrator/cmd/workflow-runner/sdk"
)

// Evaluator evaluates conditions using CEL (Common Expression Language)
type Evaluator struct {
	cache map[string]cel.Program
	mu    sync.RWMutex
}

// NewEvaluator creates a new condition evaluator with caching
func NewEvaluator() *Evaluator {
	return &Evaluator{
		cache: make(map[string]cel.Program),
	}
}

// Evaluate evaluates a condition and returns the result
func (e *Evaluator) Evaluate(condition *sdk.Condition, output interface{}, context map[string]interface{}) (bool, error) {
	if condition == nil {
		return false, fmt.Errorf("nil condition")
	}

	switch condition.Type {
	case "cel":
		return e.evaluateCEL(condition.Expression, output, context)
	default:
		return false, fmt.Errorf("unsupported condition type: %s", condition.Type)
	}
}

// evaluateCEL evaluates a CEL expression
func (e *Evaluator) evaluateCEL(expr string, output, context interface{}) (bool, error) {
	// Convert JSONPath-style $.field to CEL output.field for compatibility
	// This allows workflows to use $.approved instead of output.approved
	normalizedExpr := strings.ReplaceAll(expr, "$.", "output.")

	// Check cache first
	e.mu.RLock()
	prg, exists := e.cache[normalizedExpr]
	e.mu.RUnlock()

	if !exists {
		// Compile and cache
		var err error
		prg, err = e.compileCEL(normalizedExpr)
		if err != nil {
			return false, err
		}

		e.mu.Lock()
		e.cache[normalizedExpr] = prg
		e.mu.Unlock()
	}

	// Evaluate
	out, _, err := prg.Eval(map[string]interface{}{
		"output": output,
		"ctx":    context,
	})

	if err != nil {
		return false, fmt.Errorf("CEL evaluation error: %w", err)
	}

	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("CEL expression did not return boolean, got %T", out.Value())
	}

	return result, nil
}

// compileCEL compiles a CEL expression
func (e *Evaluator) compileCEL(expr string) (cel.Program, error) {
	// Create CEL environment with variables
	env, err := cel.NewEnv(
		cel.Variable("output", cel.DynType),
		cel.Variable("ctx", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL env: %w", err)
	}

	// Compile expression
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL compilation error: %w", issues.Err())
	}

	// Create program
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program: %w", err)
	}

	return prg, nil
}

// ClearCache clears the compiled expression cache
func (e *Evaluator) ClearCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string]cel.Program)
}

// CacheSize returns the number of cached expressions
func (e *Evaluator) CacheSize() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.cache)
}
