load("../../busybox", "busybox")


derivation(
    name="nested2",
    builder="%s/bin/sh" % busybox,
    environment={"PATH": "%s/bin" % busybox},
    sources=["../file.txt", "../script.sh"],
    args=["../script.sh"],
)

derivation(
    name="nested",
    environment={"PATH": "%s/bin" % busybox},
    builder="%s/bin/sh" % busybox,
    sources=["../"],
    args=["../script.sh"],
)
