package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		name             string
		args             []string
		selection        string
		namespace        string
		all              bool
		summaryCondition string
		wantErr          bool
	}{
		{"name and namespace", []string{"deployment/app", "-n", "staging"}, "deployment/app", "staging", false, "", false},
		{"type list", []string{"-A", "deployments,services"}, "deployments,services", "", true, "", false},
		{"summary before selection", []string{"-s", "Ready", "deployment/app"}, "deployment/app", "", false, "Ready", false},
		{"summary after selection", []string{"deployment/app", "-s", "Ready"}, "deployment/app", "", false, "Ready", false},
		{"long summary", []string{"deployment/app", "--summary-condition", "Succeeded"}, "deployment/app", "", false, "Succeeded", false},
		{"summary equals", []string{"--summary-condition=Complete", "deployment/app"}, "deployment/app", "", false, "Complete", false},
		{"short equals", []string{"deployment/app", "-s=Available"}, "deployment/app", "", false, "Available", false},
		{"summary missing value", []string{"deployment/app", "-s"}, "", "", false, "", true},
		{"summary before flag", []string{"deployment/app", "-s", "-A"}, "", "", false, "", true},
		{"summary empty value", []string{"deployment/app", "--summary-condition="}, "", "", false, "", true},
		{"summary whitespace", []string{"deployment/app", "-s", " "}, "", "", false, "", true},
		{"flags conflict", []string{"deployment/app", "-n", "staging", "-A"}, "", "", false, "", true},
		{"labels unsupported", []string{"deployments", "-l", "app=web"}, "", "", false, "", true},
		{"no selection", nil, "", "", false, "", true},
		{"multiple names", []string{"deployment/app", "service/web"}, "", "", false, "", true},
		{"mixed type list", []string{"deployment/app,service/web"}, "", "", false, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args, &bytes.Buffer{})
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseArgs(%q) error = %v, wantErr = %v", tc.args, err, tc.wantErr)
			}
			if !tc.wantErr && (got.selection != tc.selection || got.namespace != tc.namespace || got.allNamespaces != tc.all || got.summaryCondition != tc.summaryCondition) {
				t.Fatalf("parseArgs(%q) = %+v", tc.args, got)
			}
		})
	}
}

func TestRender(t *testing.T) {
	data := []byte(`{"kind":"List","items":[{"kind":"Deployment","metadata":{"name":"web","namespace":"prod"},"status":{"conditions":[{"type":"Old","status":"True","lastTransitionTime":"2025-01-01T00:00:00Z"},{"type":"Missing","message":"a\nb\tc"},{"type":"New","status":"False","reason":"Failed","lastTransitionTime":"2025-02-01T00:00:00Z"},{"type":"AlsoNew","lastTransitionTime":"2025-02-01T00:00:00Z"}]}},{"kind":"Node","metadata":{"name":"worker"}}]}`)
	resources, err := decodeResources(data)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := render(&output, resources, ""); err != nil {
		t.Fatal(err)
	}
	s := output.String()
	if !strings.HasPrefix(s, "prod/deployment/web\nTYPE ") || !strings.Contains(s, "node/worker\nNo conditions\n") {
		t.Fatalf("unexpected headings or table: %q", s)
	}
	if !(strings.Index(s, "New ") < strings.Index(s, "AlsoNew ") && strings.Index(s, "AlsoNew ") < strings.Index(s, "Old ") && strings.Index(s, "Old ") < strings.Index(s, "Missing ")) {
		t.Fatalf("conditions not sorted newest first: %q", s)
	}
	if !strings.Contains(s, `a\nb\tc`) {
		t.Fatalf("multiline message not escaped: %q", s)
	}
}

func TestRenderSummaryPerResource(t *testing.T) {
	data := []byte(`{"kind":"List","items":[{"kind":"Deployment","metadata":{"name":"ready"},"status":{"conditions":[{"type":"Ready","status":"True","lastTransitionTime":"2025-01-01T00:00:00Z"},{"type":"Succeeded","status":"True"},{"type":"Progressing","status":"True"}]}},{"kind":"Job","metadata":{"name":"done"},"status":{"conditions":[{"type":"Ready","status":"True"},{"type":"Succeeded","status":"True"},{"type":"Complete","status":"True"}]}},{"kind":"Pod","metadata":{"name":"waiting"},"status":{"conditions":[{"type":"Ready","status":"False"},{"type":"Succeeded","status":"Unknown"},{"type":"Scheduled","status":"True"}]}},{"kind":"Node","metadata":{"name":"empty"}}]}`)
	resources, err := decodeResources(data)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := render(&output, resources, "Ready"); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(output.String(), "\n\n")
	if len(parts) != 4 {
		t.Fatalf("expected four resources, got %q", output.String())
	}
	if !strings.Contains(parts[0], "Ready") || strings.Contains(parts[0], "Succeeded") || strings.Contains(parts[0], "Progressing") {
		t.Fatalf("Ready=True should be the only row: %q", parts[0])
	}
	if !strings.Contains(parts[1], "Ready") || strings.Contains(parts[1], "Succeeded") || strings.Contains(parts[1], "Complete") {
		t.Fatalf("only the selected type should be shown: %q", parts[1])
	}
	for _, kind := range []string{"Ready", "Succeeded", "Scheduled"} {
		if !strings.Contains(parts[2], kind) {
			t.Fatalf("no qualifying match should preserve all rows: %q", parts[2])
		}
	}
	if parts[3] != "node/empty\nNo conditions\n" {
		t.Fatalf("empty resource output: %q", parts[3])
	}
	output.Reset()
	if err := render(&output, resources, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Progressing") || !strings.Contains(output.String(), "Complete") {
		t.Fatalf("default view should retain all conditions: %q", output.String())
	}
	output.Reset()
	if err := render(&output, resources, "Succeeded"); err != nil {
		t.Fatal(err)
	}
	parts = strings.Split(output.String(), "\n\n")
	if strings.Contains(parts[0], "Ready") || !strings.Contains(parts[0], "Succeeded") || strings.Contains(parts[0], "Progressing") {
		t.Fatalf("Succeeded=True should be the only row: %q", parts[0])
	}
	output.Reset()
	if err := render(&output, resources, "Complete"); err != nil {
		t.Fatal(err)
	}
	parts = strings.Split(output.String(), "\n\n")
	if !strings.Contains(parts[0], "Ready") || !strings.Contains(parts[0], "Succeeded") || !strings.Contains(parts[0], "Progressing") {
		t.Fatalf("missing chosen condition should preserve all rows: %q", parts[0])
	}
	if strings.Contains(parts[1], "Ready") || strings.Contains(parts[1], "Succeeded") || !strings.Contains(parts[1], "Complete") {
		t.Fatalf("custom chosen condition should be the only row: %q", parts[1])
	}
}

func TestDecodeAndRenderEmptyResults(t *testing.T) {
	for _, data := range []string{"", `{"kind":"List","items":[]}`} {
		resources, err := decodeResources([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := render(&output, resources, ""); err != nil {
			t.Fatal(err)
		}
		if output.String() != "No resources found\n" {
			t.Fatalf("unexpected output: %q", output.String())
		}
	}
}

func TestDecodeSingleResource(t *testing.T) {
	resources, err := decodeResources([]byte(`{"kind":"Pod","metadata":{"name":"app","namespace":"default"}}`))
	if err != nil || len(resources) != 1 || resources[0].Metadata.Name != "app" {
		t.Fatalf("decodeResources() = %+v, %v", resources, err)
	}
}

func TestRunFetchOutcomes(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$2" in
  deployments) printf '%s\n' '{"kind":"List","items":[]}' ;;
  deployment/missing) exit 0 ;;
  *) printf '%s\n' 'error: the server does not have a resource type "unknown"' >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct {
		selection string
		wantCode  int
		wantOut   string
		wantErr   string
	}{
		{"deployments", 0, "No resources found\n", ""},
		{"deployment/missing", 0, "No resources found\n", ""},
		{"unknown", 1, "", "does not have a resource type"},
	} {
		t.Run(tc.selection, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{tc.selection}, &stdout, &stderr)
			if code != tc.wantCode || stdout.String() != tc.wantOut || !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("run() = (%d, %q, %q), expected (%d, %q, containing %q)", code, stdout.String(), stderr.String(), tc.wantCode, tc.wantOut, tc.wantErr)
			}
		})
	}
}
