load("../hermes-seed", "hermes_seed")

derivation(
    name="simple",
    environment={"hermes_seed": hermes_seed},
    builder="%s/bin/sh" % hermes_seed,
    args=[],
    sources=["./simple.c"],
)
