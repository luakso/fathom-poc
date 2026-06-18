package anatomy_test

import (
	"encoding/json"
	"testing"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func TestGraphJSONShape(t *testing.T) {
	g := anatomy.Graph{
		Chain:  "base",
		TxHash: "0xabc",
		Nodes: []anatomy.Node{{
			ID:        "tx:0xabc",
			Kind:      anatomy.NodeTransaction,
			Label:     "0xabc",
			Fields:    map[string]string{"block": "100"},
			Providers: []anatomy.ProviderRef{{Kind: "stats", Available: true}},
		}},
		Edges: []anatomy.Edge{{ID: "e1", Source: "a", Target: "b", Kind: "pays"}},
	}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var round anatomy.Graph
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	if round.Nodes[0].Kind != anatomy.NodeTransaction {
		t.Fatalf("kind round-trip failed: %q", round.Nodes[0].Kind)
	}
	if string(b) == "" || round.TxHash != "0xabc" {
		t.Fatalf("bad round-trip: %s", b)
	}
}
