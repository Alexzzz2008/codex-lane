package platform

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestPowerShellJSONAcceptsStatements(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell is only available on Windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result struct {
		Name  string `json:"Name"`
		Count int    `json:"Count"`
	}
	script := `
$items = @('a','b','c')
if ($items.Count -ne 3) { throw 'unexpected count' }
[pscustomobject]@{ Name = 'codex-lane'; Count = $items.Count }`
	if err := PowerShellJSON(ctx, script, &result); err != nil {
		t.Fatalf("PowerShellJSON: %v", err)
	}
	if result.Name != "codex-lane" || result.Count != 3 {
		t.Fatalf("unexpected payload: %+v", result)
	}
}

func TestPowerShellJSONKeepsEmptyArrays(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell is only available on Windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result struct {
		Items []string `json:"Items"`
	}
	if err := PowerShellJSON(ctx, `[pscustomobject]@{ Items = @() }`, &result); err != nil {
		t.Fatalf("PowerShellJSON: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected no items, got %+v", result.Items)
	}
}
