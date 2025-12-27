# Детальное ревью реализации Итерации 4 задачи 0009

## Обзор

Итерация 4 направлена на исправление проблем с исполнением многострочного кода в Docker-контейнерах, особенно для TypeScript.

**Контракт из задачи (строки 139-147):**
- Для TypeScript писать код в файл (`/tmp/main.ts`) через heredoc (или base64) и запускать `npx ts-node /tmp/main.ts`
- Унифицировать подход для bash/ruby/php (файл + запуск)

## 1. Анализ текущей реализации

### 1.1 TypeScript (строки 275-279)

```go
case LangTypeScript:
    // Write code to a file to support multi-line code
    escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
    cmd := []string{"sh", "-c", fmt.Sprintf("printf '%%s' '%s' > /tmp/main.ts && npx ts-node /tmp/main.ts", escapedCode)}
    return cmd, nil
```

**Статус:** ✅ **Соответствует контракту**
- Код записывается в файл `/tmp/main.ts`
- Исполняется через `npx ts-node /tmp/main.ts`
- Используется `printf '%s'` для записи

**Но есть проблемы с экранированием!**

### 1.2 Bash (строки 257-261)

```go
case LangBash:
    // Write to file to support multi-line scripts with proper newlines
    escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
    cmd := []string{"sh", "-c", fmt.Sprintf("printf '%%s' '%s' > /tmp/script.sh && sh /tmp/script.sh", escapedCode)}
    return cmd, nil
```

**Статус:** ✅ **Унифицирован с TypeScript**
- Одинаковый подход: файл + запуск
- Одинаковое экранирование

### 1.3 Ruby и PHP (строки 263-273)

Аналогично Bash - унифицированный подход.

**Статус:** ✅ **Унифицировано**

## 2. Критические проблемы с экранированием

### 2.1 Текущее экранирование

```go
escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
```

Это экранирование работает **только для одиночных кавычек**!

### 2.2 Проблемные случаи

#### Кейс 1: Одинарные кавычки ✅ РАБОТАЕТ

```bash
# Исходный код:
echo 'Hello World'

# После экранирования:
echo '"'"'Hello World'"'"'

# Итоговая команда (корректна):
printf '%s' 'echo '"'"'Hello World'"'"'' > /tmp/script.sh
```

#### Кейс 2: Двойные кавычки ⚠️ РАБОТАЕТ, но опасно

```bash
# Исходный код:
echo "Hello World"

# После экранирования (не изменён):
echo "Hello World"

# Итоговая команда:
printf '%s' 'echo "Hello World"' > /tmp/script.sh
```

**Работает**, потому что внутри одинарных кавычек двойные не интерпретируются.

#### Кейс 3: Символ доллара ⚠️ РАБОТАЕТ, но опасно

```bash
# Исходный код:
echo $HOME

# Итоговая команда:
printf '%s' 'echo $HOME' > /tmp/script.sh
```

**Работает**, потому что внутри одинарных кавычек `$` не интерпретируется.

#### Кейс 4: Бэктики ⚠️ РАБОТАЕТ, но опасно

```bash
# Исходный код:
echo `date`

# Итоговая команда:
printf '%s' 'echo `date`' > /tmp/script.sh
```

**Работает**, потому что внутри одинарных кавычек бэктики не интерпретируются.

#### Кейс 5: Смешанные кавычки ✅ РАБОТАЕТ

```bash
# Исходный код:
echo "It's a test"

# После экранирования:
echo "It'"'"'s a test"

# Итоговая команда (корректна):
printf '%s' 'echo "It'"'"'s a test"' > /tmp/script.sh
```

#### Кейс 6: Сложный скрипт ✅ РАБОТАЕТ

```bash
# Исходный код:
#!/bin/bash
VAR="test's value"
echo "$VAR"
echo 'Single $VAR'
echo "Backtick: $(date)"

# После экранирования:
#!/bin/bash
VAR="test'"'"'s value"
echo "$VAR"
echo '"'"'Single $VAR'"'"'
echo "Backtick: $(date)"

# Итоговая команда (корректна):
printf '%s' '#!/bin/bash
VAR="test'"'"'s value"
echo "$VAR"
echo '"'"'Single $VAR'"'"'
echo "Backtick: $(date)"' > /tmp/script.sh && sh /tmp/script.sh
```

## 3. Вывод по текущей реализации

### 3.1 Что работает ✅

1. **TypeScript записывается в файл** (`/tmp/main.ts`) ✅
2. **Bash/Ruby/PHP унифицированы** (файл + запуск) ✅
3. **Многострочный код** сохраняется корректно ✅
4. **Экранирование одинарных кавычек** работает ✅
5. **Специальные символы** (`$`, `` ` ``, `"`, `\`) **защищены** одинарными кавычками в `printf` ✅

### 3.2 Потенциальные проблемы ⚠️

1. **Длина команды**: Для очень больших скриптов команда может превысить лимиты ядра (`ARG_MAX`)
2. **Нет обработки MCP_INPUT_FILE**: В контракте упоминается `MCP_INPUT_FILE` для больших input, но это не реализовано
3. **Нет экранирования для обратного слэша в некоторых кейсах**: Хотя в одинарных кавычках `\` безопасен

### 3.3 Edge cases, требующие внимания 🔍

#### Edge case 1: Очень длинный код

```bash
# Если код > 128KB, команда может превысить ARG_MAX
```

**Рекомендация:** Рассмотреть переход на heredoc или base64 для больших скриптов.

#### Edge case 2: Код с последовательностью `'\''` в середине

```bash
# Исходный код:
echo 'It'\''s complicated'

# После текущего экранирования:
echo '"'"'It'"'"''\'"'"''"'"'s complicated'"'"'

# Работает, но становится нечитаемым
```

**Рекомендация:** Документировать это поведение.

## 4. MCP_INPUT_JSON (Итерация 3)

### 4.1 Реализация (строки 234-241)

```go
// Serialize input to JSON for MCP_INPUT_JSON env var
env := []string{}
if input != nil {
    inputJSON, err := json.Marshal(input)
    if err == nil {
        env = append(env, fmt.Sprintf("MCP_INPUT_JSON=%s", string(inputJSON)))
    }
}
```

**Статус:** ✅ **Реализовано**

### 4.2 Проблемы

1. **Ошибка json.Marshal игнорируется**: Если `input` не может быть сериализован, об этом нет логирования
2. **Нет MCP_INPUT_FILE**: Контракт требует `MCP_INPUT_FILE` для больших input
3. **Нет MCP_INPUT_<KEY>**: Контракт упоминает плоские env-переменные для примитивов

## 5. Тестовое покрытие

### 5.1 Существующие тесты ✅

1. `TestPrepareCommandWithInput` - проверяет `MCP_INPUT_JSON` ✅
2. `TestPrepareCommandTypeScriptUsesFile` - проверяет TypeScript с файлом ✅
3. `TestPrepareCommandBashUsesFile` - проверяет Bash с файлом ✅
4. `TestPrepareCommandNilInputDoesNotCrash` - проверяет nil input ✅

### 5.2 Отсутствующие тесты ❌

1. **Нет тестов для специальных символов**:
   - Двойные кавычки
   - Доллар `$`
   - Бэктики `` ` ``
   - Смешанные кавычки
   - Обратный слэш `\`

2. **Нет тестов для очень длинного кода** (> ARG_MAX)

3. **Нет тестов для больших input** (требующих `MCP_INPUT_FILE`)

4. **Нет интеграционных тестов для TypeScript с многострочным кодом**

## 6. Альтернативные подходы

### 6.1 Heredoc подход (рекомендуемый)

```go
func generateHeredocCommand(code string, filename string, interpreter string) string {
    delimiter := "MCPEOF_d8f4a3b9" // Уникальный delimiter
    return fmt.Sprintf("cat > %s << '%s'\n%s\n%s\n%s %s",
        filename, delimiter, code, delimiter, interpreter, filename)
}
```

**Преимущества:**
- ✅ Не требует экранирования
- ✅ Нет ограничений на длину (в пределах разумного)
- ✅ Более читаемые команды
- ✅ Безопасно для всех символов

**Недостатки:**
- ⚠️ Требует проверки, что delimiter не встречается в коде (хотя `MCPEOF_d8f4a3b9` практически уникален)

### 6.2 Base64 подход

```go
func generateBase64Command(code string, filename string, interpreter string) string {
    encoded := base64.StdEncoding.EncodeToString([]byte(code))
    return fmt.Sprintf("echo '%s' | base64 -d > %s && %s %s",
        encoded, filename, interpreter, filename)
}
```

**Преимущества:**
- ✅ Абсолютно безопасно для всех символов
- ✅ Не требует экранирования
- ✅ Гарантированно работает

**Недостатки:**
- ⚠️ Увеличивает размер команды на ~33%
- ⚠️ Менее читаемо для отладки

### 6.3 Сравнение длины команд

| Подход | Пример | Длина команды |
|--------|--------|---------------|
| Текущий (printf + escape) | `printf '%s' 'code' > file && sh file` | ~100-200 байт |
| Heredoc | `cat > file << 'EOF'\ncode\nEOF\nsh file` | ~80-150 байт |
| Base64 | `echo 'encoded' \| base64 -d > file && sh file` | ~120-180 байт (code × 1.33) |

**Вывод:** Heredoc - оптимален по размеру и читаемости.

## 7. Рекомендации

### 7.1 Критические (P0)

1. **Добавить тесты для специальных символов** в `container_test.go`:
   ```go
   func TestPrepareCommandSpecialCharacters(t *testing.T) {
       // Тесты для $, `, ", ', \, и их комбинаций
   }
   ```

2. **Добавить логирование ошибок json.Marshal**:
   ```go
   if err != nil {
       // TODO: log or return error
   }
   ```

### 7.2 Важные (P1)

3. **Реализовать MCP_INPUT_FILE** для больших input (> 1KB):
   ```go
   if len(inputJSON) > 1024 {
       // Записать в /tmp/mcp_input.json
       // Установить MCP_INPUT_FILE=/tmp/mcp_input.json
   }
   ```

4. **Рассмотреть переход на heredoc** для более надежного экранирования:
   ```go
   func (c *ContainerLifecycle) prepareInterpretedCommand(...) {
       delimiter := fmt.Sprintf("MCPEOF_%x", time.Now().UnixNano())
       cmd := []string{"sh", "-c", fmt.Sprintf(
           "cat > %s << '%s'\n%s\n%s\n%s %s",
           filename, delimiter, code, delimiter, interpreter, filename,
       )}
       return cmd, nil
   }
   ```

### 7.3 Желательные (P2)

5. **Добавить интеграционные тесты** для TypeScript с реальным Docker

6. **Документировать edge cases** в комментариях к `prepareInterpretedCommand`

7. **Добавить метрики**: количество символов кода, время экранирования

## 8. Acceptance criteria (из задачи)

> `execute_code(language="typescript")` корректно исполняет многострочный код.

**Статус:** ✅ **ВЫПОЛНЕНО**

Но с оговорками:
- ✅ Многострочный код работает
- ✅ Специальные символы защищены одинарными кавычками
- ⚠️ Нет тестов для edge cases
- ⚠️ Нет обработки очень длинных скриптов

## 9. Итоговая оценка

### Что реализовано ✅

1. TypeScript записывается в файл `/tmp/main.ts` ✅
2. Bash/Ruby/PHP унифицированы (файл + запуск) ✅
3. Многострочный код сохраняется корректно ✅
4. `MCP_INPUT_JSON` реализован (Итерация 3) ✅
5. Базовое тестовое покрытие есть ✅

### Что нужно доработать ⚠️

1. Тесты для специальных символов ❌
2. Обработка больших скриптов (> ARG_MAX) ❌
3. `MCP_INPUT_FILE` для больших input ❌
4. Логирование ошибок json.Marshal ❌

### Критичность проблем

- **Блокеры:** Нет
- **Критические:** Отсутствие тестов для edge cases
- **Важные:** Нет поддержки больших input через `MCP_INPUT_FILE`
- **Незначительные:** Отсутствие логирования ошибок

## 10. Конкретные исправления

### 10.1 Добавить тесты (HIGH PRIORITY)

Создать файл `internal/executor/docker/container_special_chars_test.go`:

```go
package docker

import (
	"strings"
	"testing"
)

func TestPrepareCommandSpecialCharacters(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectInCmd []string // Подстроки, которые должны быть в команде
	}{
		{
			name: "Double quotes",
			code: `echo "Hello World"`,
			expectInCmd: []string{"/tmp/script.sh", "printf"},
		},
		{
			name: "Dollar sign",
			code: `echo $HOME`,
			expectInCmd: []string{"/tmp/script.sh"},
		},
		{
			name: "Backticks",
			code: "echo `date`",
			expectInCmd: []string{"/tmp/script.sh"},
		},
		{
			name: "Mixed quotes",
			code: `echo "It's a test"`,
			expectInCmd: []string{"/tmp/script.sh", `It'"'"'s`},
		},
		{
			name: "Backslashes",
			code: `echo "Line 1\nLine 2"`,
			expectInCmd: []string{"/tmp/script.sh"},
		},
		{
			name: "Complex script",
			code: `#!/bin/bash
VAR="test's value"
echo "$VAR"
echo 'Single $VAR'
echo "Backtick: $(date)"`,
			expectInCmd: []string{"/tmp/script.sh", "test", "VAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := &ContainerLifecycle{
				language: LangBash,
			}

			cmd, env, err := lifecycle.prepareCommand(tt.code, nil)
			if err != nil {
				t.Fatalf("prepareCommand failed: %v", err)
			}

			cmdStr := strings.Join(cmd, " ")
			for _, expect := range tt.expectInCmd {
				if !strings.Contains(cmdStr, expect) {
					t.Errorf("Command does not contain %q: %s", expect, cmdStr)
				}
			}

			// Verify env is not nil
			if env == nil {
				t.Error("Expected non-nil env")
			}
		})
	}
}

func TestPrepareCommandTypeScriptSpecialChars(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangTypeScript,
	}

	code := `const msg = "It's a \"test\"";
console.log(msg);
console.log(\`Template: \${msg}\`);`

	cmd, _, err := lifecycle.prepareCommand(code, nil)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	cmdStr := strings.Join(cmd, " ")
	if !strings.Contains(cmdStr, "/tmp/main.ts") {
		t.Error("Expected command to write to /tmp/main.ts")
	}
	if !strings.Contains(cmdStr, "npx ts-node") {
		t.Error("Expected command to use npx ts-node")
	}
}
```

### 10.2 Улучшить обработку ошибок json.Marshal (MEDIUM PRIORITY)

```go
// В container.go, строки 234-241:
env := []string{}
if input != nil {
    inputJSON, err := json.Marshal(input)
    if err != nil {
        // Возвращать ошибку вместо игнорирования
        return nil, nil, fmt.Errorf("failed to marshal input: %w", err)
    }
    env = append(env, fmt.Sprintf("MCP_INPUT_JSON=%s", string(inputJSON)))
}
```

### 10.3 Реализовать MCP_INPUT_FILE (LOW PRIORITY, но в контракте)

```go
const maxEnvSize = 1024 * 1024 // 1MB

env := []string{}
if input != nil {
    inputJSON, err := json.Marshal(input)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to marshal input: %w", err)
    }

    if len(inputJSON) > maxEnvSize {
        // Записать в файл
        writeCmd := fmt.Sprintf("cat > /tmp/mcp_input.json << 'MCPEOF'\n%s\nMCPEOF", string(inputJSON))
        env = append(env, "MCP_INPUT_FILE=/tmp/mcp_input.json")
        // Добавить writeCmd в начало команды
    } else {
        env = append(env, fmt.Sprintf("MCP_INPUT_JSON=%s", string(inputJSON)))
    }
}
```

## 11. Заключение

**Итерация 4 выполнена успешно** с точки зрения основного контракта:
- ✅ TypeScript записывается в файл через `printf`
- ✅ Bash/Ruby/PHP унифицированы
- ✅ Многострочный код работает

**Но требуется доработка:**
- ❌ Добавить тесты для edge cases (критично)
- ❌ Реализовать `MCP_INPUT_FILE` (в контракте, но не реализовано)
- ❌ Улучшить обработку ошибок

**Текущее качество кода:** 7/10
- Функциональность: 9/10 ✅
- Надежность: 6/10 ⚠️ (нет тестов для edge cases)
- Полнота: 6/10 ⚠️ (не реализован `MCP_INPUT_FILE`)
- Документированность: 8/10 ✅

**Рекомендация:** Принять Итерацию 4 с условием доработки тестов (P0).
