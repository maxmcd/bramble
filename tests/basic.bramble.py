# test hardlinks and symlinks


link_test = derivation(
    name="link-test",
    builder="fetch_url",
    environment={
        "decompress": True,
        "url": "$test_url/archive.tar.gz",
        "hash": "e1161382e01e38cef645c944a1387417402cc957caa6750ff1dc6ef6fb19abf4",
    },
)
