package dbt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	dbtproject "github.com/DIZAMU/RootCause/dbt"
)

// TestBuildRefusesOtherHosts checks that a DSN for any database but the
// compose Postgres is refused before docker is called.
func TestBuildRefusesOtherHosts(t *testing.T) {
	for _, dsn := range []string{
		"postgres://u:p@db.example.com:5432/rootcause_test",
		"postgres://u:p@127.0.0.1:5433/rootcause_test",
	} {
		if err := Build(context.Background(), dsn); !errors.Is(err, ErrUnsupportedDatabase) {
			t.Errorf("%s: err = %v, want ErrUnsupportedDatabase", dsn, err)
		}
	}
}

// TestTargetOverride checks RC_DBT_NETWORK and RC_DBT_HOST: both or neither,
// and never the host network or no network.
func TestTargetOverride(t *testing.T) {
	t.Setenv(envNetwork, "ci_net")
	t.Setenv(envHost, "postgres")
	if n, h, err := target(context.Background()); err != nil || n != "ci_net" || h != "postgres" {
		t.Errorf("target = %q, %q, %v; want ci_net, postgres", n, h, err)
	}
	for _, bad := range [][2]string{{"ci_net", ""}, {"", "postgres"}, {"host", "postgres"}, {"none", "postgres"}, {"container:pg", "postgres"}} {
		t.Setenv(envNetwork, bad[0])
		t.Setenv(envHost, bad[1])
		if _, _, err := target(context.Background()); err == nil {
			t.Errorf("target with network %q host %q: no error", bad[0], bad[1])
		}
	}
}

// TestExpectedNodesOfProject pins the node count of the embedded project.
// A change to the models or tests must update it on purpose.
func TestExpectedNodesOfProject(t *testing.T) {
	n, err := ExpectedNodes(dbtproject.FS)
	if err != nil {
		t.Fatal(err)
	}
	if n != 55 {
		t.Errorf("ExpectedNodes = %d, want 55 (8 models, 3 singular tests, 44 generic tests)", n)
	}
}

// TestExpectedNodesCounts covers each place a node can come from.
func TestExpectedNodesCounts(t *testing.T) {
	fsys := fstest.MapFS{
		"dbt_project.yml":     {Data: []byte("name: x\n")},
		"models/a/m1.sql":     {Data: []byte("select 1")},
		"models/b/m2.sql":     {Data: []byte("select 1")},
		"models/b/notes.md":   {Data: []byte("not a node")},
		"tests/one_row.sql":   {Data: []byte("select 1 where false")},
		"analyses/ignore.sql": {Data: []byte("select 1")},
		"models/a/schema.yml": {Data: []byte(`
version: 2
models:
  - name: m1
    data_tests: [one_model_test]
    columns:
      - name: id
        tests: [unique, not_null]
      - name: kind
        data_tests:
          - accepted_values:
              values: [a, b]
`)},
		"models/b/sources.yaml": {Data: []byte(`
version: 2
sources:
  - name: engine
    tables:
      - name: session
        columns:
          - name: id
            tests: [not_null]
      - name: event
`)},
	}
	// 2 models + 1 singular test + 4 generic tests on m1 + 1 on a source.
	if n, err := ExpectedNodes(fsys); err != nil || n != 8 {
		t.Errorf("ExpectedNodes = %d, %v; want 8", n, err)
	}
}

// TestVerdict checks every outcome, including a run that exits 0 but ran
// fewer nodes than the project has (the review's TOTAL=36 instead of 55).
func TestVerdict(t *testing.T) {
	exit1 := exec.Command("sh", "-c", "exit 1").Run()
	full := "12:00:00  Done. PASS=55 WARN=0 ERROR=0 SKIP=0 NO-OP=0 TOTAL=55\n"
	cases := []struct {
		name           string
		ctxErr, runErr error
		output, want   string
	}{
		{"pass", nil, nil, full, ""},
		{"shrunken run", nil, nil, "Done. PASS=36 WARN=0 ERROR=0 SKIP=0 NO-OP=0 TOTAL=36\n", "ran 36 nodes, the project has 55"},
		{"no summary", nil, nil, "Encountered an error\n", "printed no node total"},
		{"exit 1", nil, exit1, full, "exited 1"},
		{"cancelled", context.Canceled, exit1, "", "cancelled"},
		{"timeout", context.DeadlineExceeded, exit1, "", "did not finish"},
	}
	for _, c := range cases {
		got := verdict(c.ctxErr, c.runErr, c.output, 55)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: verdict = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestExtract checks that the extracted project matches the embedded one and
// is readable by any user, as the userns-remapped container root needs.
func TestExtract(t *testing.T) {
	dir, err := extract(dbtproject.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	files := 0
	err = fs.WalkDir(dbtproject.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		if d.IsDir() {
			if fi.Mode().Perm() != 0o755 {
				t.Errorf("%s: mode %v, want 0755", p, fi.Mode().Perm())
			}
			return nil
		}
		files++
		if fi.Mode().Perm() != 0o644 {
			t.Errorf("%s: mode %v, want 0644", p, fi.Mode().Perm())
		}
		want, _ := fs.ReadFile(dbtproject.FS, p)
		got, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if string(got) != string(want) {
			t.Errorf("%s: extracted content differs", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("nothing extracted")
	}
	if _, err := os.Stat(filepath.Join(dir, "embed.go")); err == nil {
		t.Error("embed.go was extracted; only the dbt project belongs in the mount")
	}
}

// TestPruneLogs keeps only the newest keepLogs logs.
func TestPruneLogs(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < keepLogs+5; i++ {
		name := filepath.Join(dir, fmt.Sprintf("202601%02dT000000Z-db-x.log", i+1))
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneLogs(dir)
	entries, _ := os.ReadDir(dir)
	if len(entries) != keepLogs {
		t.Fatalf("%d logs left, want %d", len(entries), keepLogs)
	}
	if entries[0].Name() != "20260106T000000Z-db-x.log" {
		t.Errorf("oldest kept = %s, want the 6th oldest", entries[0].Name())
	}
}
