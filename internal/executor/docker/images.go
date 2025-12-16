package docker

import (
	"fmt"
)

// LanguageImages maps languages to their Docker images
var LanguageImages = map[Language]string{
	LangRuby:       "ruby:3.3-alpine",
	LangRust:       "rust:1.75-alpine",
	LangJava:       "eclipse-temurin:21-alpine",
	LangPHP:        "php:8.3-cli-alpine",
	LangBash:       "alpine:3.19",
	LangTypeScript: "node:20-alpine",
}

// LanguageCommands maps languages to their execution commands
var LanguageCommands = map[Language][]string{
	LangRuby:       {"ruby"},
	LangRust:       {"sh", "-c"},
	LangJava:       {"sh", "-c"},
	LangPHP:        {"php"},
	LangBash:       {"sh"},
	LangTypeScript: {"npx", "ts-node"},
}

// LanguageFileExtensions maps languages to their file extensions
var LanguageFileExtensions = map[Language]string{
	LangRuby:       ".rb",
	LangRust:       ".rs",
	LangJava:       ".java",
	LangPHP:        ".php",
	LangBash:       ".sh",
	LangTypeScript: ".ts",
}

// GetImage returns the Docker image for a language
func GetImage(lang Language) (string, error) {
	if NativeRuntimes[lang] {
		return "", fmt.Errorf("language %s uses native runtime, no Docker image needed", lang)
	}

	image, ok := LanguageImages[lang]
	if !ok {
		return "", fmt.Errorf("no Docker image configured for language: %s", lang)
	}

	return image, nil
}

// GetCommand returns the execution command for a language
func GetCommand(lang Language) ([]string, error) {
	cmd, ok := LanguageCommands[lang]
	if !ok {
		return nil, fmt.Errorf("no command configured for language: %s", lang)
	}

	return cmd, nil
}

// GetFileExtension returns the file extension for a language
func GetFileExtension(lang Language) string {
	ext, ok := LanguageFileExtensions[lang]
	if !ok {
		return ".txt"
	}
	return ext
}

// IsNativeRuntime checks if a language uses native runtime
func IsNativeRuntime(lang Language) bool {
	return NativeRuntimes[lang]
}

// NeedsCompilation checks if a language needs compilation before execution
func NeedsCompilation(lang Language) bool {
	switch lang {
	case LangRust, LangJava:
		return true
	default:
		return false
	}
}

// GetCompileCommand returns the compilation command for languages that need it
func GetCompileCommand(lang Language) ([]string, error) {
	switch lang {
	case LangRust:
		return []string{"rustc", "-o", "/tmp/program"}, nil
	case LangJava:
		return []string{"javac"}, nil
	default:
		return nil, fmt.Errorf("language %s does not need compilation", lang)
	}
}
