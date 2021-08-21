package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/dstruct"
	"github.com/maxmcd/dag"
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
		Builder:     fmt.Sprintf("%s/sh", first.String()),
		Args:        []string{"build_it"},
	}
	second.populateUnbuiltInputDerivations()
	store.StoreDerivation(second)

	third := &Derivation{
		store:       store,
		Name:        "scrip2",
		OutputNames: []string{"out", "foo"},
		Builder:     fmt.Sprintf("%s/sh", second.String()),
		Args:        []string{"build_it2"},
	}
	third.populateUnbuiltInputDerivations()
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

		newTemplateName := drv.String()
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
