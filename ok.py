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


build(
    name="bootstrap-tools",
    builder="fetch_url",
    environment={
        "url": "http://tarballs.nixos.org/stdenv-linux/x86_64/c5aabb0d603e2c1ea05f5a93b3be82437f5ebf31/bootstrap-tools.tar.xz",
        "hash": "a5ce9c155ed09397614646c9717fc7cd94b1023d7b76b618d409e4fefd6e9d39",
    },
)
