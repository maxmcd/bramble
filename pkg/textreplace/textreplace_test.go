package textreplace

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	radix "github.com/hashicorp/go-immutable-radix"
)

func TestMatch(t *testing.T) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	//solution := [][]byte{[]byte("/nix/store/sadfasdfasdfasdfasdfasdfasdf-foo.drv")}

	prefixToReplace := "/tmp/wings/"

	r := NewReplacer(input, []byte(prefixToReplace), GenerateRandomCorpus(input))
	fmt.Println(r.NextValue([]byte("/nix")))
	fmt.Println(r.NextValue([]byte("y")))
	fmt.Println(r.tree.Root().Maximum())
	fmt.Println(r.NextValue([]byte("y")))

	b, _ := ioutil.ReadAll(r)
	for _, in := range prepNixPaths(prefixToReplace, SomeRandomNixPaths) {
		if !bytes.Contains(b, in) {
			t.Errorf("bytes should contain %s but they do not", string(in))
		}
	}
}

func TestFrameReader(t *testing.T) {
	lorem := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Aliquam pharetra velit sit amet nibh vulputate imperdiet. Pellentesque hendrerit consequat metus.")
	expectedAnswer := bytes.Replace(lorem, []byte("dolor"), []byte("fishy"), -1)
	source := bytes.NewBuffer([]byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Aliquam pharetra velit sit amet nibh vulputate imperdiet. Pellentesque hendrerit consequat metus."))

	out := bytes.Buffer{}

	_, _ = CopyWithFrames(&out, source, 5, func(b []byte) error {
		fmt.Println(string(b))
		copy(b, bytes.Replace(b, []byte("dolor"), []byte("fishy"), -1))
		return nil
	})
	if !bytes.Equal(out.Bytes(), expectedAnswer) {
		fmt.Println(out.String())
		fmt.Println(string(expectedAnswer))
		t.Error("bytes should be fishy but they're not")
	}

}

func TestOverlap(t *testing.T) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	prefixToReplace := "/tmp/wings/"

	r := NewReplacer(input, []byte(prefixToReplace), GenerateUninterruptedCorpus(input, 100))

	_, _ = ioutil.ReadAll(r)
	if r.replacements != 100 {
		t.Error("got", r.replacements, "replacements, wanted", 100)
	}
}

func TestGen(t *testing.T) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	b, err := ioutil.ReadAll(GenerateRandomCorpus(input))
	if err != nil {
		t.Error(err)
	}
	for _, in := range input {
		if !bytes.Contains(b, in) {
			t.Errorf("bytes should contain %s but they do not", string(in))
		}
	}
}

func TestRadix(t *testing.T) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	tree := radix.New()
	for _, in := range input {
		tree, _, _ = tree.Insert(in, struct{}{})
	}
	tree.Root().WalkPrefix([]byte("/nix/store/zzi"), func(k []byte, _ interface{}) bool {
		fmt.Println(string(k))
		return false
	})
}

func BenchmarkReplace(b *testing.B) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	//solution := [][]byte{[]byte("/nix/store/sadfasdfasdfasdfasdfasdfasdf-foo.drv")}

	prefixToReplace := "/tmp/wings/"

	corpus := GenerateRandomCorpus(input)
	r := NewReplacer(input, []byte(prefixToReplace), corpus)

	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		_, _ = ioutil.ReadAll(r)
	}
}

func BenchmarkFrameReader(b *testing.B) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(input)

	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		r := ReadyByFrame(200, corpus, func(b []byte) error {
			return nil
		})
		_, _ = ioutil.ReadAll(r)
	}
}
func BenchmarkJustStream(b *testing.B) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(input)

	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		_, _ = ioutil.ReadAll(corpus)
	}
}

func BenchmarkUninterruptedReplace(b *testing.B) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	prefixToReplace := "/tmp/wings/"
	corpus := GenerateUninterruptedCorpus(input, 1000)
	r := NewReplacer(input, []byte(prefixToReplace), corpus)

	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		_, _ = ioutil.ReadAll(r)
	}
}
