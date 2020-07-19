# test hardlinks and symlinks


link_test = derivation(
    name="link-test",
    builder="fetch_url",
    environment={
        "decompress": True,
        "url": "$test_url/archive.tar.gz",
        "hash": "4477ff345dd98f37bc728c0683b39fc8c3efe235a716a9763dd44b70721f178a",
    },
)
