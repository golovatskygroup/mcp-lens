package server

import (
	"fmt"
	"strings"
	"testing"
)

type devModeInstructionsCase struct {
	env     string
	wantDev bool
}

func TestBuildInstructions_DevModeCases_Base100(t *testing.T) {
	cases := baseDevModeInstructionsCases100()
	if len(cases) != 100 {
		t.Fatalf("expected 100 cases, got %d", len(cases))
	}

	assertBuildInstructionsCases(t, cases)
}

func TestBuildInstructions_DevModeCases_Extra100(t *testing.T) {
	cases := extraDevModeInstructionsCases100()
	if len(cases) != 100 {
		t.Fatalf("expected 100 cases, got %d", len(cases))
	}

	assertBuildInstructionsCases(t, cases)
}

func assertBuildInstructionsCases(t *testing.T, cases []devModeInstructionsCase) {
	requiredSubstrings := []string{
		"read-only",
		"`query`",
		"search_tools",
		"describe_tool",
		"output.view",
		"artifact_uri",
		"resources/read",
		"Never attempt mutations",
	}
	forbiddenSubstrings := []string{
		"Available categories:",
		"Total available tools:",
		"Local tools (used internally by the router):",
	}

	for i, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("%03d", i), func(t *testing.T) {
			t.Setenv("MCP_LENS_DEV_MODE", tc.env)

			s := &Server{}
			got := s.buildInstructions()

			if len(got) > 1200 {
				t.Fatalf("instructions too long: %d bytes", len(got))
			}
			for _, sub := range requiredSubstrings {
				if !strings.Contains(got, sub) {
					t.Fatalf("missing required substring %q\ninstructions:\n%s", sub, got)
				}
			}
			for _, sub := range forbiddenSubstrings {
				if strings.Contains(got, sub) {
					t.Fatalf("found forbidden substring %q\ninstructions:\n%s", sub, got)
				}
			}

			hasDev := strings.Contains(got, "dev_scaffold_tool")
			if hasDev != tc.wantDev {
				t.Fatalf("dev mode mismatch: env=%q wantDev=%v hasDev=%v\ninstructions:\n%s", tc.env, tc.wantDev, hasDev, got)
			}
		})
	}
}

func baseDevModeInstructionsCases100() []devModeInstructionsCase {
	prefixes := []string{"", " ", "\t", "\n", "  "}
	suffixes := []string{"", " ", "\t", "\n", "  "}

	truthyTokens := []string{"1", "true", "yes"}
	cases := make([]devModeInstructionsCase, 0, 100)
	for _, tok := range truthyTokens {
		for _, pre := range prefixes {
			for _, suf := range suffixes {
				cases = append(cases, devModeInstructionsCase{env: pre + tok + suf, wantDev: true})
			}
		}
	}

	falsy := []string{
		"",
		"0",
		"00",
		"01",
		"2",
		"-1",
		" truee ",
		"tru",
		"yesplease",
		"yess",
		"y",
		"t",
		"on",
		"off",
		"enabled",
		"disabled",
		"enable",
		"disable",
		"false",
		"no",
		"nil",
		"null",
		" ",
		"\t",
		"\n",
	}
	for _, v := range falsy {
		cases = append(cases, devModeInstructionsCase{env: v, wantDev: false})
	}
	return cases
}

func extraDevModeInstructionsCases100() []devModeInstructionsCase {
	prefixes := []string{"", "\r", "\v"}
	suffixes := []string{"", "\f", "\r\n"}

	truthyTokens := []string{"TRUE", "True", "tRuE", "YES", "Yes", "yEs", "1"}
	cases := make([]devModeInstructionsCase, 0, 100)
	for _, tok := range truthyTokens {
		for _, pre := range prefixes {
			for _, suf := range suffixes {
				cases = append(cases, devModeInstructionsCase{env: pre + tok + suf, wantDev: true})
			}
		}
	}

	falsy := []string{
		"TRUE!",
		"YES!",
		"Tr ue",
		"y e s",
		"1.0",
		"1,0",
		"10",
		"1true",
		"true1",
		"yes1",
		"true\tfalse",
		"yes\nno",
		"yes\rno",
		"yes\r\nno",
		"yes\tno",
		"1\n0",
		"0\n1",
		"1\t2",
		"true\tfalse\t",
		"ye",
		"tru",
		"ttrue",
		"yyes",
		"maybe",
		"ok",
		"enable=true",
		"dev=1",
		"\"true\"",
		"'true'",
		" true\u200b",
		"\u200btrue",
		"\r\v\f",
		"\v\r\n\f",
		"true\n\rfalse",
		"yes\r\n\rno",
		"1\r\n2",
		"0\r\n1",
	}
	for _, v := range falsy {
		cases = append(cases, devModeInstructionsCase{env: v, wantDev: false})
	}

	return cases
}
