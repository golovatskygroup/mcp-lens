package testutil

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var loadOnce sync.Once

// LoadDotEnv loads variables from a ".env" file if present.
// It is intended for tests only. Existing environment variables are not overridden.
//
// Search strategy:
// - starting at the current working directory
// - walking up parent directories until ".env" is found or filesystem root is reached.
func LoadDotEnv() error {
	var errOut error
	loadOnce.Do(func() {
		path, err := findUpwards(".env")
		if err != nil {
			errOut = nil
			return
		}
		_ = loadEnvFile(path)
	})
	return errOut
}

func findUpwards(name string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := wd
	for {
		candidate := filepath.Join(dir, name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not found")
		}
		dir = parent
	}
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		// Split KEY=VALUE only on the first '='
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if key == "" {
			continue
		}
		// Strip surrounding quotes (simple .env support).
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		// Do not override existing env vars.
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		_ = os.Setenv(key, val)
	}
	return sc.Err()
}
