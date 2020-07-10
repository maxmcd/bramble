# def build_python_package(*args, **kwargs):
#     pass


# build_python_package(
#     pname="numpy",
#     version="1.18.5",
#     nativeBuildInputs=["gfortran", "pytest", "cython", "setuptoolsBuildHook"],
#     buildInputs=["blas", "lapack"],
#     checkPhase="""
#     echo hi
#     """,
# )

hermes_seed = derivation(
    name="hermes-seed",
    builder="fetch_url",
    environment={
        "decompress": "true",
        "url": "https://github.com/maxmcd/bramble/releases/download/v0.0.1/linux-x86_64-seed.tar.gz",
        "hash": "a5ce9c155ed09397614646c9717fc7cd94b1023d7b76b618d409e4fefd6e9d39",
    },
)
