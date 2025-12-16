package executor

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// Validator validates code before execution
type Validator struct {
	sandbox *SandboxConfig
}

// NewValidator creates a new code validator
func NewValidator(sandbox *SandboxConfig) *Validator {
	return &Validator{
		sandbox: sandbox,
	}
}

// Validate validates code for the given language
func (v *Validator) Validate(language Language, code string) error {
	switch language {
	case LanguageGo:
		return v.validateGo(code)
	case LanguageJavaScript:
		return v.validateJavaScript(code)
	case LanguagePython:
		return v.validatePython(code)
	default:
		return fmt.Errorf("unsupported language: %s", language)
	}
}

// validateGo validates Go code
func (v *Validator) validateGo(code string) error {
	// Parse Go code
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", code, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("Go syntax error: %w", err)
	}

	// Check for forbidden patterns
	var validationErr error
	ast.Inspect(file, func(n ast.Node) bool {
		if validationErr != nil {
			return false
		}

		switch node := n.(type) {
		case *ast.ImportSpec:
			// Check imported package
			importPath := strings.Trim(node.Path.Value, `"`)
			if v.sandbox != nil && v.sandbox.Go != nil {
				// Check forbidden packages
				for _, forbidden := range v.sandbox.Go.ForbiddenPackages {
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
						validationErr = fmt.Errorf("forbidden package: %s", importPath)
						return false
					}
				}
				// Check if package is in allowed list
				allowed := false
				for _, allowedPkg := range v.sandbox.Go.AllowedPackages {
					if importPath == allowedPkg || strings.HasPrefix(importPath, allowedPkg+"/") {
						allowed = true
						break
					}
				}
				if !allowed {
					validationErr = fmt.Errorf("package not in allowed list: %s", importPath)
					return false
				}
			}

		case *ast.CallExpr:
			// Check for dangerous function calls
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				funcName := sel.Sel.Name
				// Check for exec.Command
				if funcName == "Command" {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "exec" {
						validationErr = fmt.Errorf("forbidden function: exec.Command")
						return false
					}
				}
				// Check for os.Setenv
				if funcName == "Setenv" {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "os" {
						validationErr = fmt.Errorf("forbidden function: os.Setenv")
						return false
					}
				}
			}
		}

		return true
	})

	return validationErr
}

// validateJavaScript validates JavaScript code
func (v *Validator) validateJavaScript(code string) error {
	if v.sandbox == nil || v.sandbox.JavaScript == nil {
		return nil
	}

	// Check for forbidden patterns using regex
	forbiddenPatterns := []struct {
		pattern *regexp.Regexp
		message string
	}{
		{regexp.MustCompile(`\beval\s*\(`), "forbidden: eval()"},
		{regexp.MustCompile(`\bnew\s+Function\s*\(`), "forbidden: new Function()"},
		{regexp.MustCompile(`\brequire\s*\(`), "forbidden: require()"},
		{regexp.MustCompile(`\bimport\s+`), "forbidden: import statement"},
		{regexp.MustCompile(`\b__proto__\b`), "forbidden: __proto__"},
		{regexp.MustCompile(`\bconstructor\s*\[\s*["']constructor["']\s*\]`), "forbidden: constructor access"},
	}

	// Check forbidden globals
	for _, forbidden := range v.sandbox.JavaScript.ForbiddenGlobals {
		pattern := regexp.MustCompile(fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(forbidden)))
		forbiddenPatterns = append(forbiddenPatterns, struct {
			pattern *regexp.Regexp
			message string
		}{
			pattern: pattern,
			message: fmt.Sprintf("forbidden global: %s", forbidden),
		})
	}

	// Check all patterns
	for _, fp := range forbiddenPatterns {
		if fp.pattern.MatchString(code) {
			return fmt.Errorf("%s", fp.message)
		}
	}

	return nil
}

// validatePython validates Python code
func (v *Validator) validatePython(code string) error {
	if v.sandbox == nil || v.sandbox.Python == nil {
		return nil
	}

	// Check for forbidden patterns using regex
	forbiddenPatterns := []struct {
		pattern *regexp.Regexp
		message string
	}{
		{regexp.MustCompile(`\beval\s*\(`), "forbidden: eval()"},
		{regexp.MustCompile(`\bexec\s*\(`), "forbidden: exec()"},
		{regexp.MustCompile(`\bcompile\s*\(`), "forbidden: compile()"},
		{regexp.MustCompile(`\b__import__\s*\(`), "forbidden: __import__()"},
		{regexp.MustCompile(`\bopen\s*\(`), "forbidden: open()"},
		{regexp.MustCompile(`\binput\s*\(`), "forbidden: input()"},
		{regexp.MustCompile(`\bsubprocess\b`), "forbidden: subprocess module"},
		{regexp.MustCompile(`\bos\.system\b`), "forbidden: os.system"},
		{regexp.MustCompile(`\bos\.popen\b`), "forbidden: os.popen"},
	}

	// Check forbidden modules
	for _, forbidden := range v.sandbox.Python.ForbiddenModules {
		pattern := regexp.MustCompile(fmt.Sprintf(`\bimport\s+%s\b`, regexp.QuoteMeta(forbidden)))
		forbiddenPatterns = append(forbiddenPatterns, struct {
			pattern *regexp.Regexp
			message string
		}{
			pattern: pattern,
			message: fmt.Sprintf("forbidden module: %s", forbidden),
		})
		pattern = regexp.MustCompile(fmt.Sprintf(`\bfrom\s+%s\s+import\b`, regexp.QuoteMeta(forbidden)))
		forbiddenPatterns = append(forbiddenPatterns, struct {
			pattern *regexp.Regexp
			message string
		}{
			pattern: pattern,
			message: fmt.Sprintf("forbidden module: %s", forbidden),
		})
	}

	// Check restricted builtins
	for _, builtin := range v.sandbox.Python.RestrictedBuiltins {
		pattern := regexp.MustCompile(fmt.Sprintf(`\b%s\s*\(`, regexp.QuoteMeta(builtin)))
		forbiddenPatterns = append(forbiddenPatterns, struct {
			pattern *regexp.Regexp
			message string
		}{
			pattern: pattern,
			message: fmt.Sprintf("forbidden builtin: %s", builtin),
		})
	}

	// Check all patterns
	for _, fp := range forbiddenPatterns {
		if fp.pattern.MatchString(code) {
			return fmt.Errorf("%s", fp.message)
		}
	}

	return nil
}

// ValidateImports validates that only allowed imports are used
func (v *Validator) ValidateImports(language Language, imports []string) error {
	switch language {
	case LanguageGo:
		if v.sandbox == nil || v.sandbox.Go == nil {
			return nil
		}
		for _, imp := range imports {
			if !v.sandbox.IsPackageAllowed(imp) {
				return fmt.Errorf("package not allowed: %s", imp)
			}
		}
	case LanguagePython:
		if v.sandbox == nil || v.sandbox.Python == nil {
			return nil
		}
		for _, imp := range imports {
			if !v.sandbox.IsModuleAllowed(imp) {
				return fmt.Errorf("module not allowed: %s", imp)
			}
		}
	}
	return nil
}

// ExtractGoImports extracts import paths from Go code
func ExtractGoImports(code string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", code, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	var imports []string
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		imports = append(imports, importPath)
	}

	return imports, nil
}

// ExtractPythonImports extracts import names from Python code (simple regex-based)
func ExtractPythonImports(code string) []string {
	var imports []string

	// Match: import module
	importPattern := regexp.MustCompile(`^\s*import\s+([\w.]+)`)
	// Match: from module import ...
	fromPattern := regexp.MustCompile(`^\s*from\s+([\w.]+)\s+import`)

	lines := strings.Split(code, "\n")
	for _, line := range lines {
		if matches := importPattern.FindStringSubmatch(line); len(matches) > 1 {
			imports = append(imports, matches[1])
		}
		if matches := fromPattern.FindStringSubmatch(line); len(matches) > 1 {
			imports = append(imports, matches[1])
		}
	}

	return imports
}
