package sync

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Optional cross-repository contract check, against the actual receiving source.
// The regular suite still exercises every boundary and the real JSON wire path.
func TestProviderPreflightContract(t *testing.T) {
	root := os.Getenv("HEARTHROOM_PROVIDER_SOURCE")
	if root == "" {
		t.Skip("set HEARTHROOM_PROVIDER_SOURCE to a server/provider checkout for source contract verification")
	}
	parse := func(rel string) *ast.File {
		t.Helper()
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	var value func(ast.Expr) constant.Value
	value = func(e ast.Expr) constant.Value {
		switch n := e.(type) {
		case *ast.BasicLit:
			return constant.MakeFromLiteral(n.Value, n.Kind, 0)
		case *ast.BinaryExpr:
			if n.Op == token.SHL {
				v, _ := constant.Uint64Val(value(n.Y))
				return constant.Shift(value(n.X), n.Op, uint(v))
			}
			return constant.BinaryOp(value(n.X), n.Op, value(n.Y))
		}
		t.Fatalf("unsupported contract expression %T", e)
		return nil
	}
	checkConstants := func(rel string, wants map[string]int) {
		t.Helper()
		found := map[string]bool{}
		ast.Inspect(parse(rel), func(n ast.Node) bool {
			v, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, name := range v.Names {
				want, ok := wants[name.Name]
				if !ok {
					continue
				}
				if i >= len(v.Values) {
					t.Fatalf("constant %s no longer explicit", name.Name)
				}
				actual, ok := constant.Int64Val(value(v.Values[i]))
				if !ok || int(actual) != want {
					t.Errorf("%s: %s=%v, CLI expects %d", rel, name.Name, actual, want)
				}
				found[name.Name] = true
			}
			return true
		})
		for name := range wants {
			if !found[name] {
				t.Errorf("missing source contract constant %s", name)
			}
		}
	}
	checkConstants("internal/service/role/author_asset.go", map[string]int{"AuthorAssetMaxBytes": rulesTotalMax, "AuthorAssetRuleReplaceMax": ruleReplaceMax})
	checkConstants("internal/surface/rolesurface/trial.go", map[string]int{"trialBodyLimit": trialBodyMax})
	checkConstants("internal/service/trial/trial.go", map[string]int{"MaxEntries": 1000, "MaxNameRunes": 100, "MaxRuleReplace": ruleReplaceMax, "MaxRulesTotal": rulesTotalMax})
	checkConstants("internal/service/role/shared_state.go", map[string]int{"SharedMaxPolls": 16, "SharedMaxCounters": 16, "SharedMaxOptions": 12, "SharedMaxStep": 1000, "SharedMaxPerMember": 1000000000, "SharedMaxGoal": 1000000000000})
	found := map[string]bool{}
	ast.Inspect(parse("internal/service/role/limits.go"), func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) != 1 || v.Names[0].Name != "fieldLimits" {
			return true
		}
		m, ok := v.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("fieldLimits contract shape changed")
		}
		for _, e := range m.Elts {
			pair := e.(*ast.KeyValueExpr)
			key, _ := strconv.Unquote(pair.Key.(*ast.BasicLit).Value)
			want, ok := fieldLimits[key]
			if !ok {
				t.Errorf("new server field limit %s needs CLI preflight", key)
				continue
			}
			vals := pair.Value.(*ast.CompositeLit)
			if len(vals.Elts) != 2 {
				t.Fatalf("%s shape changed", key)
			}
			for i, expr := range vals.Elts {
				actual, _ := constant.Int64Val(value(expr))
				if int(actual) != want[i] {
					t.Errorf("%s locale %d: server %d, CLI %d", key, i, actual, want[i])
				}
			}
			found[key] = true
		}
		return false
	})
	for key := range fieldLimits {
		if !found[key] {
			t.Errorf("missing source contract for %s", key)
		}
	}
}
