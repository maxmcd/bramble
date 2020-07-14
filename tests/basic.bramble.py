# test hardlinks and symlinks


link_test = derivation(
    name="link-test",
    builder="fetch_url",
    environment={
        "decompress": True,
        "url": "$test_url/archive.tar.gz",
        "hash": "0cb30c37bdb22f19cbfc23bf5a1a7a38bcd3ea449bf7c62534332fc259cd7810",
    },
)
