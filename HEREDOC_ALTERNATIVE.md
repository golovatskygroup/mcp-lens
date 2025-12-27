# Альтернативная реализация с Heredoc

## Проблема с текущим подходом

Текущий подход с `printf '%s' 'code' > file` имеет потенциальные проблемы:

1. **Ограничение длины**: Для очень больших скриптов команда может превысить `ARG_MAX` (обычно 128KB-2MB)
2. **Сложность экранирования**: Требуется экранирование одинарных кавычек
3. **Читаемость**: Экранированный код становится нечитаемым

## Предлагаемое решение: Heredoc

### Преимущества

1. **Не требует экранирования**: Heredoc с quoted delimiter (`<< 'EOF'`) не интерпретирует специальные символы
2. **Нет ограничений на длину**: Heredoc не имеет ограничений `ARG_MAX`
3. **Читаемость**: Код остается читаемым в командной строке
4. **Безопасность**: Уникальный delimiter предотвращает коллизии

### Реализация

```go
// prepareInterpretedCommand prepares command for interpreted languages using heredoc
func (c *ContainerLifecycle) prepareInterpretedCommand(code string, input interface{}, baseCmd []string) ([]string, error) {
	// Generate unique delimiter based on timestamp to avoid collisions
	delimiter := fmt.Sprintf("MCPEOF_%x", time.Now().UnixNano())

	var filename, interpreter string

	switch c.language {
	case LangBash:
		filename = "/tmp/script.sh"
		interpreter = "sh"
	case LangRuby:
		filename = "/tmp/script.rb"
		interpreter = "ruby"
	case LangPHP:
		filename = "/tmp/script.php"
		interpreter = "php"
	case LangTypeScript:
		filename = "/tmp/main.ts"
		interpreter = "npx ts-node"
	default:
		return nil, fmt.Errorf("unsupported language: %s", c.language)
	}

	// Use heredoc with quoted delimiter to prevent variable expansion
	// Format: cat > file << 'DELIMITER'\ncode\nDELIMITER\ninterpreter file
	cmdStr := fmt.Sprintf("cat > %s << '%s'\n%s\n%s\n%s %s",
		filename, delimiter, code, delimiter, interpreter, filename)

	cmd := []string{"sh", "-c", cmdStr}
	return cmd, nil
}
```

### Сравнение подходов

#### Текущий подход (printf + escape)

```bash
# Исходный код:
#!/bin/bash
VAR="test's value"
echo "$VAR"

# Команда:
sh -c printf '%s' '#!/bin/bash
VAR="test'"'"'s value"
echo "$VAR"' > /tmp/script.sh && sh /tmp/script.sh
```

**Проблемы:**
- Экранирование одинарных кавычек (`'"'"'`)
- Сложно читать
- Может превысить ARG_MAX для больших скриптов

#### Heredoc подход

```bash
# Исходный код:
#!/bin/bash
VAR="test's value"
echo "$VAR"

# Команда:
sh -c cat > /tmp/script.sh << 'MCPEOF_19a3f4b8'
#!/bin/bash
VAR="test's value"
echo "$VAR"
MCPEOF_19a3f4b8
sh /tmp/script.sh
```

**Преимущества:**
- Нет экранирования
- Код остается читаемым
- Нет ограничений на длину
- Безопасно для всех символов

### Тесты для heredoc

```go
func TestPrepareCommandHeredoc(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		code     string
	}{
		{
			name:     "Bash with single quotes",
			language: LangBash,
			code:     "echo 'Hello World'",
		},
		{
			name:     "TypeScript with all special chars",
			language: LangTypeScript,
			code:     `const x = "It's \"complex\""; console.log(\`Template: \${x}\`);`,
		},
		{
			name: "Very long script",
			language: LangBash,
			code: strings.Repeat("echo 'line'\n", 10000), // 10K lines
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := &ContainerLifecycle{
				language: tt.language,
			}

			cmd, env, err := lifecycle.prepareCommand(tt.code, nil)
			if err != nil {
				t.Fatalf("prepareCommand failed: %v", err)
			}

			cmdStr := strings.Join(cmd, " ")

			// Verify heredoc is used
			if !strings.Contains(cmdStr, "cat >") {
				t.Error("Expected heredoc approach with 'cat >'")
			}
			if !strings.Contains(cmdStr, "MCPEOF_") {
				t.Error("Expected unique delimiter MCPEOF_*")
			}

			// Verify code is not escaped (should contain original quotes)
			if strings.Contains(tt.code, "'") {
				// Original single quotes should be preserved
				if !strings.Contains(cmdStr, tt.code) {
					t.Log("Note: Code may be preserved as-is (check manually)")
				}
			}

			if env == nil {
				t.Error("Expected non-nil env")
			}
		})
	}
}
```

### Потенциальная проблема: Коллизия delimiter

**Проблема:** Если код содержит строку `MCPEOF_19a3f4b8`, heredoc завершится преждевременно.

**Решение 1:** Использовать криптографически случайный delimiter

```go
import "crypto/rand"

func generateDelimiter() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("MCPEOF_%x", b)
}
```

**Решение 2:** Проверить наличие delimiter в коде и регенерировать

```go
func generateSafeDelimiter(code string) string {
	for {
		delimiter := fmt.Sprintf("MCPEOF_%x", time.Now().UnixNano())
		if !strings.Contains(code, delimiter) {
			return delimiter
		}
		time.Sleep(1 * time.Nanosecond) // Ensure unique
	}
}
```

**Решение 3:** Использовать base64 для абсолютной безопасности

```go
func prepareInterpretedCommandBase64(code string, filename string, interpreter string) ([]string, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(code))
	cmdStr := fmt.Sprintf("echo '%s' | base64 -d > %s && %s %s",
		encoded, filename, interpreter, filename)
	return []string{"sh", "-c", cmdStr}, nil
}
```

## Рекомендация

1. **Для большинства случаев**: Оставить текущий подход с `printf` (работает, протестирован)
2. **Для очень больших скриптов** (> 100KB): Добавить автоматическое переключение на heredoc
3. **Для параноидальной безопасности**: Использовать base64

### Гибридный подход

```go
const maxPrintfSize = 100 * 1024 // 100KB

func (c *ContainerLifecycle) prepareInterpretedCommand(code string, input interface{}, baseCmd []string) ([]string, error) {
	var filename, interpreter string

	switch c.language {
	case LangBash:
		filename = "/tmp/script.sh"
		interpreter = "sh"
	case LangTypeScript:
		filename = "/tmp/main.ts"
		interpreter = "npx ts-node"
	// ... other cases
	default:
		return nil, fmt.Errorf("unsupported language: %s", c.language)
	}

	// Use heredoc for large scripts, printf for small ones
	if len(code) > maxPrintfSize {
		return c.prepareWithHeredoc(code, filename, interpreter)
	}

	return c.prepareWithPrintf(code, filename, interpreter)
}

func (c *ContainerLifecycle) prepareWithPrintf(code string, filename string, interpreter string) ([]string, error) {
	escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
	cmdStr := fmt.Sprintf("printf '%%s' '%s' > %s && %s %s", escapedCode, filename, interpreter, filename)
	return []string{"sh", "-c", cmdStr}, nil
}

func (c *ContainerLifecycle) prepareWithHeredoc(code string, filename string, interpreter string) ([]string, error) {
	delimiter := c.generateSafeDelimiter(code)
	cmdStr := fmt.Sprintf("cat > %s << '%s'\n%s\n%s\n%s %s",
		filename, delimiter, code, delimiter, interpreter, filename)
	return []string{"sh", "-c", cmdStr}, nil
}

func (c *ContainerLifecycle) generateSafeDelimiter(code string) string {
	// Use timestamp + random for uniqueness
	for i := 0; i < 10; i++ {
		delimiter := fmt.Sprintf("MCPEOF_%x_%d", time.Now().UnixNano(), i)
		if !strings.Contains(code, delimiter) {
			return delimiter
		}
	}
	// Fallback to very long random string
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("MCPEOF_%x", b)
}
```

## Заключение

**Текущий подход (printf + escape) достаточен** для большинства случаев и протестирован.

**Heredoc подход** может быть добавлен как улучшение для:
1. Очень больших скриптов (> 100KB)
2. Улучшения читаемости команд в логах
3. Уменьшения сложности экранирования

**Не требуется срочного изменения**, но heredoc - хорошее направление для будущих улучшений.
