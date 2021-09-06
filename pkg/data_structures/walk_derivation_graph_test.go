package ds

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testDO struct {
	hash string
	name string
}

func (tdo testDO) Hash() string {
	return tdo.hash
}
func (tdo testDO) Output() string {
	return tdo.name
}

var _ DerivationOutput = testDO{}

type testDerivation struct {
	name         string
	built        bool
	dependencies []string
}

var _ DrvReplacable = new(testDerivation)

func (td *testDerivation) String() string {
	t := "f"
	if td.built {
		t = "t"
	}
	start := fmt.Sprintf("(%s %s", td.name, t)
	end := ")"
	if len(td.dependencies) != 0 {
		end = fmt.Sprintf(" (%s)", strings.Join(td.dependencies, " ")) + end
	}
	return start + end
}

func (td *testDerivation) Hash() string {
	return td.String()
}

func (td *testDerivation) build() {
	td.built = true
}
func (td *testDerivation) ReplaceHash(old, new string) {
	for i, dep := range td.dependencies {
		if dep == old {
			td.dependencies[i] = new
		}
	}
}

func TestWalkDerivationGraphSingle(t *testing.T) {
	// This isn't a very good test. The behavior of testDerivation is a bit
	// wrong since dependencies should be derevation outputs and not
	// derivations. Good to visually confirm that this thing works but not very
	// good otherwise.
	a := &testDerivation{name: "a"}
	aHash := a.Hash()
	aDO := testDO{hash: aHash, name: "a:out"}

	b := &testDerivation{
		name:         "b",
		dependencies: []string{aHash},
	}
	bDO := testDO{hash: b.Hash(), name: "b:out"}

	c := &testDerivation{name: "c"}
	cHash := c.Hash()
	cDO := testDO{hash: cHash, name: "c:out"}

	d := &testDerivation{
		name:         "d",
		dependencies: []string{aHash, b.Hash(), cHash},
	}
	dDO := testDO{hash: d.Hash(), name: "d:out"}

	dStringPrev := d.String()
	_ = dStringPrev
	require.NoError(t, WalkDerivationGraph(WalkDerivationGraphOptions{
		Edges: []DerivationGraphEdge{
			NewDerivationGraphEdge(bDO, aDO),
			NewDerivationGraphEdge(dDO, aDO),
			NewDerivationGraphEdge(dDO, bDO),
			NewDerivationGraphEdge(dDO, cDO),
		},
		Derivations: []DrvReplacable{a, b, c, d}},
		func(do DerivationOutput, drv DrvReplacable) (newHash string, err error) {
			drv.(*testDerivation).build()
			return drv.Hash(), nil
		},
	))
	assert.NotContains(t, d.String(), "false")
	fmt.Println(dStringPrev)
	fmt.Println(d.String())
}
