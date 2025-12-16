package tools

import "testing"

func TestConfluenceXhtmlToText_Basic(t *testing.T) {
	got, truncated, err := confluenceXhtmlToText(`<p>Hello <strong>world</strong></p><p>Next<br/>Line</p>`, false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated {
		t.Fatalf("expected not truncated")
	}
	want := "Hello world\nNext\nLine"
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestConfluenceXhtmlToText_List(t *testing.T) {
	got, _, err := confluenceXhtmlToText(`<ul><li>One</li><li>Two</li></ul>`, false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "- One\n- Two"
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestConfluenceXhtmlToText_CDATAPlainTextBody(t *testing.T) {
	got, _, err := confluenceXhtmlToText(`<ac:plain-text-body><![CDATA[line1
line2]]></ac:plain-text-body>`, false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nline2"
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestConfluenceXhtmlToText_PreserveLinks(t *testing.T) {
	got, _, err := confluenceXhtmlToText(`<p><a href="https://ex.com">link</a></p>`, true, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "link (https://ex.com)"
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%q\ngot:\n%q", want, got)
	}
}
