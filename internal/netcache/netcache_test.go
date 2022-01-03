package netcache

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDO(t *testing.T) {
	t.Skip("this test requires live credentials")
	client, err := NewS3Cache(S3CacheOptions{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		S3url:           "https://nyc3.digitaloceanspaces.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	fakeFile := make([]byte, 1e7)
	if _, err := rand.Read(fakeFile); err != nil {
		t.Fatal(err)
	}

	key := "test/testfile@+-$%^"
	{
		// put
		writer, err := client.Put(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(writer, bytes.NewBuffer(fakeFile)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}

	{
		// get
		reader, err := client.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, reader); err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		require.Equal(t, fakeFile, buf.Bytes())
	}
}
