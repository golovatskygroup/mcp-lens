package docker

import (
	"testing"
)

func TestGetImage(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		want     string
		wantErr  bool
	}{
		{
			name:     "Ruby image",
			language: LangRuby,
			want:     "ruby:3.3-alpine",
			wantErr:  false,
		},
		{
			name:     "Rust image",
			language: LangRust,
			want:     "rust:1.75-alpine",
			wantErr:  false,
		},
		{
			name:     "Java image",
			language: LangJava,
			want:     "eclipse-temurin:21-alpine",
			wantErr:  false,
		},
		{
			name:     "PHP image",
			language: LangPHP,
			want:     "php:8.3-cli-alpine",
			wantErr:  false,
		},
		{
			name:     "Bash image",
			language: LangBash,
			want:     "alpine:3.19",
			wantErr:  false,
		},
		{
			name:     "TypeScript image",
			language: LangTypeScript,
			want:     "node:20-alpine",
			wantErr:  false,
		},
		{
			name:     "Native runtime (Go) should fail",
			language: LangGo,
			want:     "",
			wantErr:  true,
		},
		{
			name:     "Native runtime (JavaScript) should fail",
			language: LangJavaScript,
			want:     "",
			wantErr:  true,
		},
		{
			name:     "Native runtime (Python) should fail",
			language: LangPython,
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetImage(tt.language)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCommand(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		wantErr  bool
	}{
		{
			name:     "Ruby command",
			language: LangRuby,
			wantErr:  false,
		},
		{
			name:     "PHP command",
			language: LangPHP,
			wantErr:  false,
		},
		{
			name:     "Bash command",
			language: LangBash,
			wantErr:  false,
		},
		{
			name:     "TypeScript command",
			language: LangTypeScript,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetCommand(tt.language)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) == 0 {
				t.Errorf("GetCommand() returned empty command")
			}
		})
	}
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		want     string
	}{
		{
			name:     "Ruby extension",
			language: LangRuby,
			want:     ".rb",
		},
		{
			name:     "Rust extension",
			language: LangRust,
			want:     ".rs",
		},
		{
			name:     "Java extension",
			language: LangJava,
			want:     ".java",
		},
		{
			name:     "PHP extension",
			language: LangPHP,
			want:     ".php",
		},
		{
			name:     "Bash extension",
			language: LangBash,
			want:     ".sh",
		},
		{
			name:     "TypeScript extension",
			language: LangTypeScript,
			want:     ".ts",
		},
		{
			name:     "Unknown language",
			language: "unknown",
			want:     ".txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetFileExtension(tt.language)
			if got != tt.want {
				t.Errorf("GetFileExtension() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNativeRuntime(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		want     bool
	}{
		{
			name:     "Go is native",
			language: LangGo,
			want:     true,
		},
		{
			name:     "JavaScript is native",
			language: LangJavaScript,
			want:     true,
		},
		{
			name:     "Python is native",
			language: LangPython,
			want:     true,
		},
		{
			name:     "Ruby is not native",
			language: LangRuby,
			want:     false,
		},
		{
			name:     "Rust is not native",
			language: LangRust,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNativeRuntime(tt.language)
			if got != tt.want {
				t.Errorf("IsNativeRuntime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsCompilation(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		want     bool
	}{
		{
			name:     "Rust needs compilation",
			language: LangRust,
			want:     true,
		},
		{
			name:     "Java needs compilation",
			language: LangJava,
			want:     true,
		},
		{
			name:     "Ruby doesn't need compilation",
			language: LangRuby,
			want:     false,
		},
		{
			name:     "PHP doesn't need compilation",
			language: LangPHP,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsCompilation(tt.language)
			if got != tt.want {
				t.Errorf("NeedsCompilation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCompileCommand(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		wantErr  bool
	}{
		{
			name:     "Rust compile command",
			language: LangRust,
			wantErr:  false,
		},
		{
			name:     "Java compile command",
			language: LangJava,
			wantErr:  false,
		},
		{
			name:     "Ruby doesn't have compile command",
			language: LangRuby,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetCompileCommand(tt.language)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCompileCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) == 0 {
				t.Errorf("GetCompileCommand() returned empty command")
			}
		})
	}
}

func TestLanguageImages_Coverage(t *testing.T) {
	// Test that all non-native languages have images
	for lang := range LanguageCommands {
		if IsNativeRuntime(lang) {
			continue
		}

		_, err := GetImage(lang)
		if err != nil {
			t.Errorf("Language %s has command but no image", lang)
		}
	}
}

func TestLanguageCommands_Coverage(t *testing.T) {
	// Test that all images have commands
	for lang := range LanguageImages {
		_, err := GetCommand(lang)
		if err != nil {
			t.Errorf("Language %s has image but no command", lang)
		}
	}
}
