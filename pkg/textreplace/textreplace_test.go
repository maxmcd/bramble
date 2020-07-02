package textreplace

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

var SomeRandomNixPaths = []string{
	"zzinxg9s1libj0k7gn7s4sl6zvvc2aja-libiec61883-1.2.0.tar.gz.drv",
	"zziylsdvcqgwwwhbspf1agbz0vldxjr3-perl5.30.2-JSON-4.02",
	"zzjsli12acp352n9i5db89hy5nnvfsdw-bcrypt-3.1.7.tar.gz.drv",
	"zzl9m6p4qzczcyf4s73n8aczrdw2ws5r-libsodium-1.0.18.tar.gz.drv",
	"zznqyjs1mz3i4ipg7cfjn0mki9ca9jvk-libxml2-2.9.10-bin",
	"zzp24m9jbrlqjp1zqf5n3s95jq6fhiqy-python3.7-python3.7-pytest-runner-4.4.drv",
	"zzsfwzjxvkvp3qmak8pwi05z99hihyng-curl-7.64.0.drv",
	"zzw8bb3ihq0jwhmy1mvcf48c3k627xbs-ghc-8.6.5-binary.drv",
	"zzxz9hl1zxana8f66jr7d7xkdhx066pm-xterm-353.drv",
	"zzy2an4hplsl06dfl6dgik4zmn7vycvd-hscolour-1.24.4.drv",
}

func prepNixPaths(prefix string, v []string) (out [][]byte) {
	for _, path := range v {
		out = append(out, []byte(prefix+path))
	}
	return
}

func GenerateUninterruptedCorpus(values [][]byte, count int) io.Reader {
	var buf bytes.Buffer
	for i := 0; i < count; i++ {
		_, _ = buf.Write(values[i%len(values)])
	}
	return bytes.NewReader(buf.Bytes())
}

func GenerateRandomCorpus(values [][]byte) io.Reader {
	count := 50
	chunks := count / (len(values))
	valueIndex := 0
	var buf bytes.Buffer
	b := make([]byte, 1024)
	for i := 0; i < count; i++ {
		rand.Read(b)
		if i%chunks == 0 {
			valueLen := len(values[valueIndex])
			// input can't be larger than the byte array
			// will panic if input size is larger than the byte array
			copy(b[0:valueLen], values[valueIndex])
			valueIndex++
		}
		buf.Write(b)
	}
	return bytes.NewReader(buf.Bytes())
}
func TestFrameReader(t *testing.T) {
	lorem := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Aliquam pharetra velit sit amet nibh vulputate imperdiet. Pellentesque hendrerit consequat metus.")
	expectedAnswer := bytes.Replace(lorem, []byte("dolor"), []byte("fishy"), -1)
	source := bytes.NewBuffer([]byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Aliquam pharetra velit sit amet nibh vulputate imperdiet. Pellentesque hendrerit consequat metus."))

	out := bytes.Buffer{}

	_, _ = CopyWithFrames(&out, source, make([]byte, 15), 5, func(b []byte) error {
		copy(b, bytes.Replace(b, []byte("dolor"), []byte("fishy"), -1))
		return nil
	})

	assert.Equal(t, out.Bytes(), expectedAnswer)
}

func TestOverlap(t *testing.T) {
	values := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	var buf bytes.Buffer
	replacements, _, err := ReplaceStrings(values, []byte("/nix/store/"), []byte("/tmp/wings/"), GenerateUninterruptedCorpus(values, 100), &buf)
	if err != nil {
		t.Error(err)
	}
	if replacements != 100 {
		t.Error("got", replacements, "replacements, wanted", 100)
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

func BenchmarkFrameReader(b *testing.B) {
	input := prepNixPaths("/nix/store/", SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(input)

	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		var buf bytes.Buffer
		_, _ = CopyWithFrames(&buf, corpus, nil, 100, func(b []byte) error {
			return nil
		})
	}
}

func BenchmarkReplace(b *testing.B) {
	values := prepNixPaths("/nix/store/", SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(values)

	var buf bytes.Buffer
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		_, _, _ = ReplaceStrings(
			values,
			[]byte("/nix/store/"),
			[]byte("/tmp/wings/"),
			corpus, &buf)
		buf.Truncate(0)
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
	values := prepNixPaths("/nix/store/", SomeRandomNixPaths)
	corpus := GenerateUninterruptedCorpus(values, 1000)

	var buf bytes.Buffer
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		_, _, _ = ReplaceStrings(
			values,
			[]byte("/nix/store/"),
			[]byte("/tmp/wings/"),
			corpus, &buf)
		buf.Truncate(0)
	}
}
