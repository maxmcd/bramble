package textreplace

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"strings"
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

func prepNixPaths(v []string) (out []string) {
	for _, path := range v {
		out = append(out, "/nix/store/"+path)
	}
	return
}

func GenerateUninterruptedCorpus(values []string, count int) io.Reader {
	var buf bytes.Buffer
	for i := 0; i < count; i++ {
		_, _ = buf.WriteString(values[i%len(values)])
	}
	return bytes.NewReader(buf.Bytes())
}

func GenerateRandomCorpus(values []string) io.Reader {
	count := 50
	chunks := count / (len(values))
	valueIndex := 0
	var buf bytes.Buffer
	b := make([]byte, 1024)
	for i := 0; i < count; i++ {
		_, _ = rand.Read(b)
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
	expectedAnswer := bytes.ReplaceAll(lorem, []byte("dolor"), []byte("fishy"))
	source := bytes.NewBuffer(lorem)

	out := bytes.Buffer{}

	_, _ = CopyWithFrames(source, &out, make([]byte, 15), 5, func(b []byte) error {
		copy(b, bytes.ReplaceAll(b, []byte("dolor"), []byte("fishy")))
		return nil
	})

	assert.Equal(t, out.Bytes(), expectedAnswer)
}

func ExampleReplaceStringsPrefix() {
	var output bytes.Buffer
	_, _, _ = ReplaceStringsPrefix(
		bytes.NewBuffer([]byte(
			"something/nix/store/zziylsdvcqgwwwhbspf1agbz0vldxjr3-perl5.30.2-JSON-4.02something"),
		),
		&output,
		[]string{"/nix/store/zziylsdvcqgwwwhbspf1agbz0vldxjr3-perl5.30.2-JSON-4.02"},
		"/nix/store/",
		"/tmp/wings/",
	)

	fmt.Println(output.String())
	// Output: something/tmp/wings/zziylsdvcqgwwwhbspf1agbz0vldxjr3-perl5.30.2-JSON-4.02something
}

func ExampleCopyWithFrames() {
	// We'd like to replace "dolor" with "fishy" in this text
	lorem := []byte("Lorem ipsum dolor sit amet")

	// With a buffer size of 15 we would split the input into
	// []byte("Lorem ipsum dol") and []byte("or sit amet")
	// and miss the opportunity to replace "dolor"
	bufferSize := 15
	expectedAnswer := bytes.ReplaceAll(lorem, []byte("dolor"), []byte("fishy"))
	source := bytes.NewBuffer(lorem)

	out := bytes.Buffer{}

	// if we set an overlap size of 5 we'll ensure we see all length 5 segments of text
	_, _ = CopyWithFrames(source, &out, make([]byte, bufferSize), 5, func(b []byte) error {
		fmt.Println(string(b))
		copy(b, bytes.ReplaceAll(b, []byte("dolor"), []byte("fishy")))
		return nil
	})
	fmt.Println(bytes.Equal(out.Bytes(), expectedAnswer))

	// Output: Lorem ipsu
	//  ipsum dolor si
	// hy sit amety si
	// true
}

// Hasher is used to compute path hash values. Hasher implements io.Writer and
// takes a sha256 hash of the input bytes. The output string is a lowercase
// base32 representation of the first 160 bits of the hash
type Hasher struct {
	hash hash.Hash
}

func NewHasher() *Hasher {
	return &Hasher{
		hash: sha256.New(),
	}
}

func (h *Hasher) Write(b []byte) (n int, err error) {
	return h.hash.Write(b)
}

func (h *Hasher) String() string {
	return bytesToBase32Hash(h.hash.Sum(nil))
}

func (h *Hasher) Sha256Hex() string {
	return fmt.Sprintf("%x", h.hash.Sum(nil))
}

// bytesToBase32Hash copies nix here
// https://nixos.org/nixos/nix-pills/nix-store-paths.html
// Finally the comments tell us to compute the base32 representation of the
// first 160 bits (truncation) of a sha256 of the above string:
func bytesToBase32Hash(b []byte) string {
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(b[:20])
	return strings.ToLower(buf.String())
}

func TestNoop(t *testing.T) {
	var buf bytes.Buffer
	b := make([]byte, 100)
	for i := 0; i < 1000; i++ {
		_, _ = rand.Read(b)
		buf.Write(b)
	}
	hasher := NewHasher()
	hasher2 := NewHasher()

	var buf2 bytes.Buffer
	written, _ := io.Copy(io.MultiWriter(&buf2, hasher), &buf)
	if written == 0 {
		t.Fatal("nope")
	}
	_, _, err := ReplaceStringsPrefix(&buf2, hasher2, []string{"adfadsfasdfasdfasdf"}, "/hi", "/ho")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hasher.String(), hasher2.String())
}

func TestOverlap(t *testing.T) {
	values := prepNixPaths(SomeRandomNixPaths)
	var buf bytes.Buffer
	replacements, _, err := ReplaceStringsPrefix(GenerateUninterruptedCorpus(values, 100), &buf, values, "/nix/store/", "/tmp/wings/")
	if err != nil {
		t.Error(err)
	}
	if replacements != 100 {
		t.Error("got", replacements, "replacements, wanted", 100)
	}
}

func TestGen(t *testing.T) {
	input := prepNixPaths(SomeRandomNixPaths)
	b, err := ioutil.ReadAll(GenerateRandomCorpus(input))
	if err != nil {
		t.Error(err)
	}
	for _, in := range input {
		if !bytes.Contains(b, []byte(in)) {
			t.Errorf("bytes should contain %s but they do not", string(in))
		}
	}
}

func BenchmarkFrameReader(b *testing.B) {
	input := prepNixPaths(SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(input)

	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		var buf bytes.Buffer
		_, _ = CopyWithFrames(corpus, &buf, nil, 100, func(b []byte) error {
			return nil
		})
	}
}

func BenchmarkReplace(b *testing.B) {
	b.StopTimer()
	values := prepNixPaths(SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(values)

	var buf bytes.Buffer
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		b.SetBytes(int64(corpus.(*bytes.Reader).Len()))
		_, _, _ = ReplaceStringsPrefix(
			corpus, &buf,
			values,
			"/nix/store/",
			"/tmp/wings/")
		buf.Reset()
	}
}

func BenchmarkReplaceReplacer(b *testing.B) {
	b.StopTimer()
	values := prepNixPaths(SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(values)

	var buf bytes.Buffer
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		b.SetBytes(int64(corpus.(*bytes.Reader).Len()))
		_ = replaceStringsPrefixReplacer(
			corpus, &buf,
			values,
			"/nix/store/",
			"/tmp/wings/")
		buf.Reset()
	}
}

func BenchmarkJustStream(b *testing.B) {
	input := prepNixPaths(SomeRandomNixPaths)

	corpus := GenerateRandomCorpus(input)

	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		b.SetBytes(int64(corpus.(*bytes.Reader).Len()))
		_, _ = ioutil.ReadAll(corpus)
	}
}

func BenchmarkUninterruptedReplace(b *testing.B) {
	b.StopTimer()
	values := prepNixPaths(SomeRandomNixPaths)
	corpus := GenerateUninterruptedCorpus(values, 1000)

	var buf bytes.Buffer
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		b.SetBytes(int64(corpus.(*bytes.Reader).Len()))
		_, _, _ = ReplaceStringsPrefix(
			corpus, &buf,
			values,
			"/nix/store/",
			"/tmp/wings/")
		buf.Reset()
	}
}

func BenchmarkUninterruptedReplaceReplacer(b *testing.B) {
	b.StopTimer()
	values := prepNixPaths(SomeRandomNixPaths)
	corpus := GenerateUninterruptedCorpus(values, 1000)

	var buf bytes.Buffer
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		b.SetBytes(int64(corpus.(*bytes.Reader).Len()))
		_ = replaceStringsPrefixReplacer(
			corpus, &buf,
			values,
			"/nix/store/",
			"/tmp/wings/")
		buf.Reset()
	}
}

func BenchmarkUninterruptedReplaceBytesReplace(b *testing.B) {
	b.StopTimer()
	values := prepNixPaths(SomeRandomNixPaths)
	corpus := GenerateUninterruptedCorpus(values, 1000)
	buildDirPrefix := []byte("zzsfwzjxvkvp3qmak8pwi05z99hihyng")
	nullBytes := make([]byte, len(buildDirPrefix))
	var buf bytes.Buffer
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		b.SetBytes(int64(corpus.(*bytes.Reader).Len()))
		_, _ = replaceBytesReplace(
			corpus, &buf,
			buildDirPrefix,
			nullBytes)
		buf.Reset()
	}
}

func TestReplaceBytes(t *testing.T) {
	values := prepNixPaths(SomeRandomNixPaths)
	corpus := GenerateUninterruptedCorpus(values, 1000)

	buildDirPrefix := []byte("zzsfwzjxvkvp3qmak8pwi05z99hihyng")
	c, _ := ioutil.ReadAll(corpus)
	assert.Equal(t, bytes.Count(c, buildDirPrefix), 100)

	nullBytes := make([]byte, len(buildDirPrefix))
	var buf bytes.Buffer
	_, _ = corpus.(*bytes.Reader).Seek(0, 0)
	_, _ = ReplaceBytes(
		corpus, &buf,
		buildDirPrefix,
		nullBytes)
	assert.Equal(t,
		bytes.ReplaceAll(buf.Bytes(), buildDirPrefix, nullBytes),
		buf.Bytes(),
	)
}

func BenchmarkUninterruptedReplaceBytes(b *testing.B) {
	b.StopTimer()
	values := prepNixPaths(SomeRandomNixPaths)
	corpus := GenerateUninterruptedCorpus(values, 1000)
	buildDirPrefix := []byte("zzsfwzjxvkvp3qmak8pwi05z99hihyng")
	nullBytes := make([]byte, len(buildDirPrefix))
	var buf bytes.Buffer
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, _ = corpus.(*bytes.Reader).Seek(0, 0)
		b.SetBytes(int64(corpus.(*bytes.Reader).Len()))
		_, _ = ReplaceBytes(
			corpus, &buf,
			buildDirPrefix,
			nullBytes)
		buf.Reset()
	}
}
