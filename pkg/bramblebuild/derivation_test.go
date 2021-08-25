package bramblebuild

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/dstruct"
	"github.com/maxmcd/dag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDerivationOutputChange(t *testing.T) {
	store, err := NewStore("")
	require.NoError(t, err)

	first := &Derivation{
		store:       store,
		Name:        "fetch_url",
		OutputNames: []string{"out"},
		Builder:     "fetch_url",
		Env:         map[string]string{"url": "1"},
	}
	store.StoreDerivation(first)

	second := &Derivation{
		store:       store,
		Name:        "script",
		OutputNames: []string{"out"},
		Builder:     fmt.Sprintf("%s/sh", first.TemplateString()),
		Args:        []string{"build_it"},
	}
	second.PopulateUnbuiltInputDerivations()
	store.StoreDerivation(second)

	third := &Derivation{
		store:       store,
		Name:        "scrip2",
		OutputNames: []string{"out", "foo"},
		Builder:     fmt.Sprintf("%s/sh", second.TemplateString()),
		Args:        []string{"build_it2"},
	}
	third.PopulateUnbuiltInputDerivations()
	store.StoreDerivation(third)

	graph, err := third.BuildDependencyGraph()
	require.NoError(t, err)
	require.NoError(t, graph.Validate())

	// We pretend to build
	counter := 1
	graph.Walk(func(v dag.Vertex) error {
		if v == dstruct.FakeDAGRoot {
			return nil
		}
		do := v.(DerivationOutput)
		drv := store.derivations.Load(do.Filename)
		drv.lock.Lock()
		defer drv.lock.Unlock()

		// We construct the template value using the DerivationOutput which
		// uses the initial value
		oldTemplateName := fmt.Sprintf(UnbuiltDerivationOutputTemplate, do.Filename, do.OutputName)

		// Build
		{
			if drv.containsUnbuiltDerivationTemplateStrings() {
				panic(drv.PrettyJSON())
			}
			// At this point it's safe to check if we've built the derivation before
			exists, err := drv.PopulateOutputsFromStore()
			require.NoError(t, err)
			_ = exists // don't build if it does

			// Fake build
			drv.Outputs = []Output{{Path: strings.Repeat(fmt.Sprint(counter), 32)}}

			// Replace outputs with correct output path
			_, err = drv.CopyWithOutputValuesReplaced()
			require.NoError(t, err)
		}

		newTemplateName := drv.TemplateString()
		for _, edge := range graph.EdgesTo(v) {
			if edge.Source() == dstruct.FakeDAGRoot {
				continue
			}
			childDO := edge.Source().(DerivationOutput)
			drv := store.derivations.Load(childDO.Filename)
			if err := drv.replaceValueInDerivation(oldTemplateName, newTemplateName); err != nil {
				panic(err)
			}
		}
		// Left to do
		// re-store derivations in store
		// clear invalid derivations?

		counter++
		return nil
	})
}

func TestDerivationValueReplacement(t *testing.T) {
	store, err := NewStore("")
	require.NoError(t, err)

	fetchURL := &Derivation{
		store:       store,
		OutputNames: []string{"out"},
		Builder:     "fetch_url",
		Env:         map[string]string{"url": "1"},
	}
	assert.Equal(t, "{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv:out }}", fetchURL.TemplateString())

	other := &Derivation{
		store:       store,
		OutputNames: []string{"out"},
		Builder:     "/bin/sh",
		Env:         map[string]string{"foo": "bar"},
	}

	building := &Derivation{
		store:       store,
		OutputNames: []string{"out"},
		Builder:     fetchURL.TemplateString() + "/bin/sh",
		Env:         map[string]string{"PATH": other.OutputTemplateString("out") + "/bin"},
	}
	// Assemble our derivations

	// Pretend we built ancestors by filling in their outputs
	fetchURL.Outputs = []Output{{Path: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	other.Outputs = []Output{{Path: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}

	store.StoreDerivation(fetchURL)
	store.StoreDerivation(other)

	building.PopulateUnbuiltInputDerivations()
	store.StorePath = "/bramble/store"
	buildCopy, err := store.copyDerivationWithOutputValuesReplaced(building)
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, buildCopy.PrettyJSON(), "/bramble/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/bin/sh")
	assert.Contains(t, buildCopy.PrettyJSON(), "/bramble/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/bin")
}
